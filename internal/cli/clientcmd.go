package cli

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// defaultClientBindings are the built-in subcommand -> local-client mappings,
// matched against connections by Type by default.
func defaultClientBindings() map[string]ClientBinding {
	return map[string]ClientBinding{
		"psql":  {Type: "postgres", Cli: "psql"},
		"redis": {Type: "redis", Cli: "redis-cli"},
	}
}

// effectiveClientBindings merges user-configured bindings over the built-in
// defaults field-by-field, so e.g. overriding just Cli for "psql" keeps its
// default Type, and a new name adds an entirely new subcommand.
func effectiveClientBindings(cfg *Config) map[string]ClientBinding {
	merged := defaultClientBindings()
	for name, override := range cfg.ClientBindings {
		base := merged[name]
		if override.Type != "" {
			base.Type = override.Type
		}
		if override.Cli != "" {
			base.Cli = override.Cli
		}
		if override.Path != "" {
			base.Path = override.Path
		}
		merged[name] = base
	}
	return merged
}

// matchesBinding reports whether conn should show up under binding's
// subcommand: its connection type matches exactly.
func matchesBinding(conn connectionInfo, binding ClientBinding) bool {
	return conn.Type == binding.Type
}

// resolveBinaryPath resolves and validates the real client binary for a
// binding: Path if set, else Cli looked up on PATH. Also returns the
// candidate name/path that was tried, for use in error messages.
func resolveBinaryPath(binding ClientBinding) (resolved string, candidate string, err error) {
	candidate = binding.Cli
	if binding.Path != "" {
		candidate = binding.Path
	}
	resolved, err = exec.LookPath(candidate)
	return resolved, candidate, err
}

// NewClientBindingCommands returns one cobra command per configured client
// binding (e.g. "psql", "redis"), each opening a single-use tunnel and
// launching the bound local CLI client against it.
func NewClientBindingCommands() []*cobra.Command {
	cfg, err := LoadConfig()
	if err != nil {
		cfg = &Config{}
	}

	bindings := effectiveClientBindings(cfg)
	names := make([]string, 0, len(bindings))
	for name := range bindings {
		names = append(names, name)
	}
	sort.Strings(names)

	var cmds []*cobra.Command
	for _, name := range names {
		n, b := name, bindings[name]
		c := &cobra.Command{
			Use:   n + " [identifier]",
			Short: fmt.Sprintf("Connect to a %s connection via %s", b.Type, b.Cli),
			Long: fmt.Sprintf(`Open a single-use tunnel to a %s connection and launch %s against it.

With no identifier, shows an interactive picker over matching connections.`, b.Type, b.Cli),
			Args: cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return runClientBinding(cmd, n, b, args)
			},
		}
		c.Flags().StringP("name", "n", "", "Connection identifier (alternative to the positional argument)")
		c.Flags().BoolP("verbose", "v", false, "Show connection progress (fetching connections, connecting, tunnel status)")
		cmds = append(cmds, c)
	}
	return cmds
}

func runClientBinding(cmd *cobra.Command, name string, binding ClientBinding, args []string) error {
	// Commands are registered (and their ClientBinding captured) before
	// cobra parses flags, so a --config override wouldn't otherwise be
	// reflected here. Re-derive the effective binding fresh now that flags
	// (and so the real config path) are known. Falls back to the
	// registration-time binding if the config can't be loaded at all.
	if cfg, cfgErr := LoadConfig(); cfgErr == nil {
		if fresh, ok := effectiveClientBindings(cfg)[name]; ok {
			binding = fresh
		}
	}

	// Validate the client binary exists before doing anything else - never
	// fail silently deep inside the tunnel/handshake logic.
	binPath, candidate, err := resolveBinaryPath(binding)
	if err != nil {
		return fmt.Errorf("%q not found (needed for '%s'): install it, make sure it's on PATH, or set client_bindings.%s.path in %s",
			candidate, name, name, GetConfigPath())
	}

	nameFlag, _ := cmd.Flags().GetString("name")
	if nameFlag != "" && len(args) == 1 {
		return fmt.Errorf("specify the connection either as --name or as a positional argument, not both")
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	vprintln := func(a ...interface{}) {
		if verbose {
			fmt.Fprintln(os.Stderr, a...)
		}
	}
	vprintf := func(format string, a ...interface{}) {
		if verbose {
			fmt.Fprintf(os.Stderr, format, a...)
		}
	}

	ctx, err := resolveContext(cmd)
	if err != nil {
		return fmt.Errorf("not logged in: %w. Please run 'login' first", err)
	}
	apiURL := resolveAPIURL(cmd, ctx.Name, ctx.APIURL)
	token := ctx.Token
	if err := validateToken(token); err != nil {
		return fmt.Errorf("authentication expired or invalid: %w\nPlease login again", err)
	}

	vprintf("Fetching connections from %s...\n", apiURL)
	connections, err := fetchConnections(apiURL, token)
	if err != nil {
		return err
	}
	var matched []connectionInfo
	for _, c := range connections {
		if matchesBinding(c, binding) {
			matched = append(matched, c)
		}
	}

	identifier := nameFlag
	if len(args) == 1 {
		identifier = args[0]
	}
	if identifier != "" {
		found := false
		for _, c := range matched {
			if c.Name == identifier {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("connection %q doesn't match '%s' (expected %s)", identifier, name, matchDescription(binding))
		}
	} else {
		if len(matched) == 0 {
			return fmt.Errorf("no connections match '%s' (expected %s)", name, matchDescription(binding))
		}
		names := make([]string, len(matched))
		for i, c := range matched {
			names[i] = c.Name
		}
		identifier, err = pickConnection(names)
		if err != nil {
			return err
		}
	}

	vprintf("Connecting to %s...\n", identifier)
	connResp, err := connectToBackend(apiURL, token, identifier)
	if err != nil {
		return err
	}

	// Open the proxy tunnel first and confirm *that* is reachable before
	// spawning the client: if the API/auth/websocket path itself is down,
	// we know immediately and clearly, instead of finding out only after
	// the client is already sitting at a prompt with a dead connection.
	// This doesn't prove the backend (Postgres/Redis) itself is up - the
	// server only dials that once the client sends its protocol handshake -
	// so a backend-side failure still surfaces from the client itself.
	vprintln("Opening tunnel...")
	preflightConn, err := dialProxyWebSocket(apiURL, token, connResp.ConnectionID)
	if err != nil {
		return fmt.Errorf("could not open tunnel to %s: %w", identifier, err)
	}
	_ = preflightConn.Close()
	vprintln("Proxy tunnel reachable. Starting " + binding.Cli + "...")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to open local listener: %w", err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port

	// Accept every local connection the client opens for the lifetime of
	// this session, not just the first one: e.g. psql's default
	// sslmode=prefer opens an initial connection for the SSL probe, and if
	// that's abandoned (a normal part of the negotiation, not a failure) it
	// opens a second, plain one to actually carry the session.
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleLocalConnection(conn, connResp.ConnectionID, token, apiURL)
		}
	}()

	clientArgs := buildClientArgs(port, token, connResp)
	clientCmd := exec.Command(binPath, clientArgs...)
	clientCmd.Stdin = os.Stdin
	clientCmd.Stdout = os.Stdout
	clientCmd.Stderr = os.Stderr
	clientCmd.Env = buildClientEnv(connResp)
	setProcessGroup(clientCmd)

	if err := clientCmd.Start(); err != nil {
		_ = listener.Close()
		<-acceptDone
		return fmt.Errorf("failed to start %s: %w", binding.Cli, err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	waitDone := make(chan error, 1)
	go func() { waitDone <- clientCmd.Wait() }()

	// The client shares our foreground process group, so it already receives
	// Ctrl+C directly and handles it itself (e.g. psql cancels the
	// in-flight query and stays open) - the first signal here is just that
	// same delivery landing on us too, not a request to kill the session.
	// Only a second signal, while we're still waiting, means the user
	// actually wants out.
	var waitErr error
	sawSignal := false
waitLoop:
	for {
		select {
		case <-sigChan:
			if !sawSignal {
				sawSignal = true
				continue waitLoop
			}
			terminateProcessGroup(clientCmd)
			select {
			case <-waitDone:
			case <-time.After(3 * time.Second):
				killProcessGroupNow(clientCmd)
				<-waitDone
			}
			_ = listener.Close()
			<-acceptDone
			os.Exit(130) // conventional SIGINT exit code
		case waitErr = <-waitDone:
			_ = listener.Close()
			<-acceptDone
			break waitLoop
		}
	}

	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		os.Exit(exitErr.ExitCode())
	}
	return waitErr
}

func matchDescription(binding ClientBinding) string {
	return fmt.Sprintf("type %q", binding.Type)
}

// buildClientArgs constructs the real client's args pointing at the local
// tunnel endpoint, filling in whatever identity/database info the backend's
// actual protocol type needs.
func buildClientArgs(port int, token string, connResp *connectResponse) []string {
	args := []string{"-h", "127.0.0.1", "-p", strconv.Itoa(port)}
	switch connResp.Type {
	case "postgres":
		username, _ := getUsernameFromToken(token)
		args = append(args, "-U", username, "-d", connResp.Database)
	}
	return args
}

// buildClientEnv returns the environment for the exec'd client, pre-filling
// whatever credential the backend's protocol needs so it never has to
// prompt interactively - the proxy already authenticated the real identity
// via the JWT and ignores whatever value this carries.
func buildClientEnv(connResp *connectResponse) []string {
	switch connResp.Type {
	case "postgres":
		return append(os.Environ(), "PGPASSWORD=proxy-managed")
	default:
		return nil
	}
}

// pickConnection shows an interactive picker (fzf if available, else a
// plain numbered menu) over names, kubens-style.
func pickConnection(names []string) (string, error) {
	if len(names) == 1 {
		return names[0], nil
	}

	if fzfPath, lookErr := exec.LookPath("fzf"); lookErr == nil {
		selected, fzfErr := runFzf(fzfPath, names)
		if fzfErr != nil {
			return "", fmt.Errorf("selection cancelled")
		}
		return selected, nil
	}

	return pickFromMenu(names)
}

func runFzf(fzfPath string, names []string) (string, error) {
	cmd := exec.Command(fzfPath, "--prompt", "connection> ")
	cmd.Stdin = strings.NewReader(strings.Join(names, "\n"))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	selected := strings.TrimSpace(string(out))
	if selected == "" {
		return "", fmt.Errorf("no selection made")
	}
	return selected, nil
}

func pickFromMenu(names []string) (string, error) {
	fmt.Println("\nSelect a connection:")
	for i, name := range names {
		fmt.Printf("  [%d] %s\n", i+1, name)
	}
	fmt.Print("Enter number: ")

	// Read exactly one line byte-by-byte rather than via a buffered reader:
	// os.Stdin is handed to the client process right after this, and a
	// buffered reader can over-read past the newline into a pipe, silently
	// stealing input meant for the child.
	line, err := readLine(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read selection: %w", err)
	}
	idx, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || idx < 1 || idx > len(names) {
		return "", fmt.Errorf("invalid selection")
	}
	return names[idx-1], nil
}

func readLine(r io.Reader) (string, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return string(line), nil
			}
			line = append(line, buf[0])
		}
		if err != nil {
			if err == io.EOF && len(line) > 0 {
				return string(line), nil
			}
			return "", err
		}
	}
}
