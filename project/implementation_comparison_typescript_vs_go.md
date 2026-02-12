# Architectural Decision Record: Implementation Options for 30M Subscribers

## Context
This document summarizes the trade-offs between the current TypeScript (Fastify) implementation and potential rewrites in Go or Java for the Entitlement Configuration Server (ECS), specifically addressing a scale of 30 million subscribers.

---

## 1. Why TypeScript was chosen initially
*   **Protocol Correctness (TS.43):** Fastify + TypeBox provides high-performance JSON Schema validation (Ajv) out of the box. The protocol's strict parameter requirements are handled at the framework level.
*   **Developer Velocity:** The "Builder Pattern" used for the 12 service handlers (ap2003-ap2016) is highly expressive in TypeScript, allowing for rapid implementation of complex business logic.
*   **Observability:** Mature OpenTelemetry auto-instrumentation for the Node.js ecosystem (pg, ioredis, http) provided immediate deep visibility.

---

## 2. Compelling Reasons for a Go Rewrite
*   **Concurrency Density:** Go’s goroutines allow a single instance to handle hundreds of concurrent cryptographic handshakes (EAP-AKA) across all CPU cores without blocking the event loop.
*   **Resource Efficiency:** Go requires significantly less RAM (256MB vs 1GB+ for Node.js) and produces smaller container images (faster cold starts).
*   **Database Connection Management:** Go can handle higher request density per instance, significantly reducing the total number of concurrent connections to PostgreSQL.
*   **Predictable Latency:** Go's garbage collector is optimized for low-latency networking, reducing the risk of "p99" spikes during high-load periods.

---

## 3. Scaling Patterns (Cloud Run)

| Metric | TypeScript (Node.js) | Go | Java (Quarkus/Native) |
| :--- | :--- | :--- | :--- |
| **Instance Size** | Small (1 vCPU / 2GB) | Beefy (4 vCPU / 1GB) | Beefy (4 vCPU / 1GB) |
| **Max Concurrency** | Low (50–80) | High (250–1000) | High (250–1000) |
| **Scaling Strategy** | Fleet of small instances | Fleet of dense instances | Fleet of dense instances |
| **Memory Footprint** | High (~1GB+) | **Lowest** (~128MB) | Low (~256MB) |
| **Cold Start** | Fast | **Instant** | Fast (with Native Image) |

---

## 4. Dimensioning for 30 Million Subscribers

### The Traffic Math (RPS)

Assume 30M subscribers have the following behavior based on GSMA TS.43:

* **Periodic Check:** Every device checks entitlements once every 24 hours.
* **Triggered Check:** Devices check on reboot, toggle of Airplane Mode, or SIM changes (assume 10% of users do this daily).
* **Successive Requests:** One "check" often involves 2 round-trips for EAP-AKA.

### The Calculation:

* 33,000,000 checks / 86,400 seconds = ~380 Requests Per Second (RPS) average.
* **Busy Hour Peak:** Assuming peaks at 3x to 5x the average.
* **Target Capacity:** System should be dimensioned for ~1,200 to 2,000 Peak RPS.

### The Redis Bottleneck (The Real Scalability Limit)

At 30M subscribers, Redis becomes a very critical component.

* **Fast Re-auth Identities:** To avoid hitting the HSS/Database, you store re-auth identities in Redis.
* **Memory Dimensioning:** 30M identities × ~500 bytes per entry (keys, counters, timestamps) ≈ 15GB of RAM.
* **Dimensioning Rule:** You need a high-memory Redis tier (e.g., Google Cloud Memorystore 20GB+).
* **TypeScript vs. Go:** The language doesn't change the memory need, but a Go client is often more efficient at managing large connection multiplexing to a single Redis instance.

### Database Dimensioning (The "Source of Truth")

* **Storage:** 30M subscribers + 30M entitlements + audit logs will quickly reach 100GB+.
* **Throughput:** At 2,000 Peak RPS, your DB will face heavy read pressure for token validation.
* **Dimensioning Rule:** Use Read Replicas. Your Cloud Run instances should point to a Load Balanced Read Proxy (like PgBouncer) or use Read Replicas for 90% of traffic (entitlement lookups), leaving the Primary for writes (token issuance).

### Language-Specific Instance Dimensioning

To handle 2,000 Peak RPS, here is how you would dimension the Cloud Run fleet:

**The TypeScript Approach (Fleet of Small Instances)**
* **Instance Size:** 1 vCPU / 2GB RAM.
* **Concurrency per instance:** 80.
* **RPS per instance:** ~150 RPS.
* **Total Instances:** ~15 to 20 instances.
* **Risk:** 20 instances each maintaining a pool of 5 DB connections = 100 concurrent DB connections. This is manageable but starts to strain smaller

### Traffic Assumptions
*   **Average Load:** ~380 Requests Per Second (RPS) based on daily checks.
*   **Peak Load (Busy Hour):** ~1,200 to 2,000 RPS (assuming 3x-5x peak).
*   **Redis Storage:** ~20GB+ RAM needed for 30M re-auth identities (~500 bytes per entry).
*   **Database Storage:** 100GB+ for subscribers, entitlements, and tokens.

### Infrastructure Target (Production)
1.  **App Tier (Go/Java):** ~4 instances, 4 vCPU / 1GB RAM each.
2.  **Redis:** 20GB+ High-Availability (Memorystore Standard Tier).
3.  **Database:** PostgreSQL (v16+) with at least 1 Read Replica and PgBouncer for connection pooling.
4.  **Observability:** Audit logs moved to BigQuery to prevent transactional DB bloat.

---

## 5. The Java Alternative (The "Industrial" Option)
Java remains the gold standard due to its robust ecosystem and low-level networking capabilities.

### Key Advantages
*   **Virtual Threads (Project Loom):** Java 21+ Virtual Threads provide concurrency density identical to Go's goroutines, allowing thousands of simultaneous handshakes on a single core.
*   **Netty Ecosystem:** Access to Netty, the world's most powerful async networking library, which powers the world's largest telecom gateways.
*   **Native Compilation:** Using **Quarkus** or **Micronaut** with **GraalVM Native Image** allows Java to achieve Go-like cold starts and memory efficiency.

### Trade-offs
*   **Complexity:** Higher verbosity and more complex build tooling compared to Go.
*   **Learning Curve:** Requires expertise in JVM tuning and GraalVM constraints for native image compilation.
*   **Suitability:** Best if the organization has existing Java/SRE expertise or requires integration with legacy Java-based telecom OSS/BSS systems.
