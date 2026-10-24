# Kafka Cluster Connection Example

## Overview
The Docker Compose setup includes a 3-node Kafka cluster (KRaft mode, no Zookeeper needed) that you can connect to using the `kafka` tag.

## Docker Services

The cluster consists of:
- **kafka-1** → `localhost:9092`
- **kafka-2** → `localhost:9093`
- **kafka-3** → `localhost:9094`

All nodes are configured with:
- KRaft mode (no Zookeeper)
- Replication factor: 3
- Min ISR: 2
- Cluster ID: `MkU3OEVBNTcwNTJENDM2Qk`

## Configuration

Each Kafka node has these tags:
```yaml
tags:
  - env:test
  - type:kafka
  - kafka
```

## Connecting to All Kafka Nodes

### Using the `kafka` Tag

Connect to all 3 Kafka nodes simultaneously:

```bash
# Start all Kafka nodes at once
port-authorizing connect --tag kafka --base-port 9092

# Output:
# 🔍 Finding connections with tag: kafka
# ✓ Found 3 connection(s)
# 
# 🚀 Starting 3 connection(s):
#   • kafka-1 → localhost:9092
#   • kafka-2 → localhost:9093
#   • kafka-3 → localhost:9094
# 
# ✓ kafka-1 connected (expires: ...)
# ✓ kafka-2 connected (expires: ...)
# ✓ kafka-3 connected (expires: ...)
```

Now you can use Kafka clients to connect to all 3 brokers:

```bash
# Kafka producer
kafka-console-producer --bootstrap-server localhost:9092,localhost:9093,localhost:9094 --topic test

# Kafka consumer
kafka-console-consumer --bootstrap-server localhost:9092,localhost:9093,localhost:9094 --topic test --from-beginning

# Create topic with replication
kafka-topics --bootstrap-server localhost:9092 --create --topic my-topic --partitions 3 --replication-factor 3

# List topics
kafka-topics --bootstrap-server localhost:9092 --list

# Describe topic
kafka-topics --bootstrap-server localhost:9092 --describe --topic my-topic
```

## Alternative: Manual Connection

You can also connect to nodes individually:

```bash
# Connect to specific nodes
port-authorizing connect kafka-1 9092 kafka-2 9093

# Or just one node
port-authorizing connect kafka-1 9092
```

## Starting the Cluster

```bash
# Start the full Docker environment
docker-compose up -d

# Wait for Kafka nodes to be healthy (takes ~30s)
docker-compose ps

# Check Kafka logs
docker-compose logs -f kafka-1 kafka-2 kafka-3

# Verify cluster is ready
docker exec port-auth-kafka-1 kafka-broker-api-versions --bootstrap-server localhost:9092
```

## Testing the Cluster

### 1. Start Port-Authorizing Proxies

```bash
# Terminal 1: Connect to all Kafka nodes
./bin/port-authorizing login
./bin/port-authorizing connect --tag kafka --base-port 9092
```

### 2. Create a Topic

```bash
# Terminal 2: Create replicated topic
kafka-topics --bootstrap-server localhost:9092,localhost:9093,localhost:9094 \
  --create \
  --topic test-topic \
  --partitions 6 \
  --replication-factor 3

# Verify topic
kafka-topics --bootstrap-server localhost:9092 --describe --topic test-topic
```

### 3. Produce Messages

```bash
# Terminal 3: Start producer
kafka-console-producer --bootstrap-server localhost:9092,localhost:9093,localhost:9094 --topic test-topic

> Hello Kafka!
> Testing replication
> Message 3
```

### 4. Consume Messages

```bash
# Terminal 4: Start consumer
kafka-console-consumer --bootstrap-server localhost:9092,localhost:9093,localhost:9094 \
  --topic test-topic \
  --from-beginning

# Should see:
# Hello Kafka!
# Testing replication
# Message 3
```

## High Availability Testing

### Test Node Failure

```bash
# Stop one node
docker stop port-auth-kafka-3

# Cluster should still work (2/3 nodes = quorum)
kafka-console-producer --bootstrap-server localhost:9092,localhost:9093 --topic test-topic
> Still working!

# Restart node
docker start port-auth-kafka-3

# Node will rejoin cluster and catch up
```

### Check Cluster Status

```bash
# Check broker list
kafka-broker-api-versions --bootstrap-server localhost:9092

# Check consumer groups
kafka-consumer-groups --bootstrap-server localhost:9092 --list

# Check topic health
kafka-topics --bootstrap-server localhost:9092 --describe --topic test-topic
```

## Connection Limits

Each Kafka connection has a 2-hour duration by default (configured in `docker/config.yaml`):

```yaml
- name: kafka-1
  type: tcp
  host: kafka-1
  port: 9092
  duration: 2h  # Auto-disconnect after 2 hours
```

You can modify this in the config if needed.

## Cleaning Up

```bash
# Stop all containers
docker-compose down

# Remove volumes (deletes all Kafka data)
docker-compose down -v
```

## Use Cases

1. **Multi-broker producer/consumer testing**
   - Test replication
   - Test partition distribution
   - Test consumer group rebalancing

2. **High availability testing**
   - Simulate broker failures
   - Test failover behavior
   - Verify data replication

3. **Development environment**
   - Full Kafka cluster locally
   - No need for cloud resources
   - Proxy through port-authorizing for audit/security

4. **Load testing**
   - Distribute load across brokers
   - Test throughput with multiple connections
   - Verify cluster performance

## Tips

1. **Use all 3 brokers in connection string** for better load balancing:
   ```
   localhost:9092,localhost:9093,localhost:9094
   ```

2. **Set proper replication factor** for topics:
   ```bash
   --replication-factor 3  # Use all nodes
   ```

3. **Monitor with JMX** (if needed):
   ```bash
   # Add JMX ports to docker-compose.yml if monitoring is required
   ```

4. **Use Kafka UI tools** like:
   - Conduktor
   - Kafka Tool
   - RedPanda Console (can be added to docker-compose)

5. **Tag-based connection is fastest**:
   ```bash
   # One command to connect to entire cluster
   pa connect --tag kafka --base-port 9092
   ```

