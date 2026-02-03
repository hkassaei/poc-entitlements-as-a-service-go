# Architecture Diagrams

## System Architecture (GCP Deployment)

```mermaid
graph TB
    subgraph Internet
        Device["Mobile Device<br/>(SIM + EAP-AKA Client)"]
    end

    subgraph GCP["Google Cloud Platform"]
        subgraph Edge["Edge Network"]
            Armor["Cloud Armor<br/>DDoS + WAF + Rate Limiting"]
            LB["Global External Application<br/>Load Balancer<br/>(HTTPS / Managed TLS)"]
        end

        subgraph VPC["VPC (Private Network)"]
            subgraph ECS_Service["Cloud Run: entitlement-server (Public Ingress via LB)"]
                ECS["ECS<br/>Fastify + TypeBox<br/>EAP-AKA State Machine<br/>Token Management<br/>13 Service Handlers"]
            end

            subgraph HSS_Service["Cloud Run: mock-hss (Internal-Only Ingress)"]
                HSS["Mock HSS<br/>MILENAGE Algorithm<br/>Envelope Encryption<br/>POST /vectors"]
            end

            Redis["Memorystore<br/>(Redis)<br/>EAP-AKA Sessions (90s TTL)<br/>Token Cache<br/>Idempotency Cache"]
            CloudSQL["Cloud SQL<br/>(PostgreSQL)<br/>Subscribers, Tokens,<br/>Entitlements, Audit Log,<br/>Encrypted Ki/OP"]
            KMS["Cloud KMS<br/>KEK (Key Encryption Key)<br/>Never leaves hardware"]
        end

        SecretMgr["Secret Manager<br/>DATABASE_URL, REDIS_URL"]
        Logging["Cloud Logging<br/>(Pino JSON auto-ingested)"]
        Trace["Cloud Trace<br/>(OpenTelemetry)"]
        Monitoring["Cloud Monitoring<br/>Metrics + Dashboards + Alerts"]
        Artifact["Artifact Registry<br/>(Container Images)"]
        Build["Cloud Build<br/>(CI/CD Pipeline)"]
    end

    Device -->|HTTPS| Armor
    Armor --> LB
    LB -->|HTTP :8080| ECS

    ECS -->|"Token R/W<br/>Session R/W<br/>Idempotency Cache"| Redis
    ECS -->|"Token CRUD<br/>Entitlement Lookup<br/>Audit Log"| CloudSQL
    ECS -->|"POST /vectors<br/>{imsi, sqn}<br/>(VPC Internal)"| HSS

    HSS -->|"Read encrypted Ki/OP"| CloudSQL
    HSS -->|"Unwrap DEK<br/>(IAM scoped to<br/>mock-hss only)"| KMS

    ECS -.->|Structured Logs| Logging
    ECS -.->|Spans| Trace
    ECS -.->|Metrics| Monitoring
    HSS -.->|Structured Logs| Logging
    HSS -.->|Spans| Trace

    Build -->|"Deploy"| ECS_Service
    Build -->|"Deploy"| HSS_Service
    Build -->|"Push Images"| Artifact

    style Armor fill:#ff6b6b,color:#fff
    style KMS fill:#ffd93d,color:#000
    style HSS fill:#6bcb77,color:#000
    style ECS fill:#4d96ff,color:#fff
    style Redis fill:#ff922b,color:#fff
    style CloudSQL fill:#845ef7,color:#fff
```

## EAP-AKA Authentication Flow

```mermaid
sequenceDiagram
    participant D as Mobile Device
    participant A as Cloud Armor + LB
    participant E as ECS (Cloud Run)
    participant R as Redis
    participant H as Mock HSS (Internal)
    participant P as PostgreSQL
    participant K as Cloud KMS

    Note over D,K: Round Trip 1: Challenge

    D->>A: POST /entitlement (IMEI, AppID, terminal_*)
    A->>E: Forward (after DDoS/WAF check)
    E->>R: Check token (none present)
    R-->>E: MISS

    E->>H: POST /vectors {imsi, sqn}
    H->>P: Read ki_encrypted, op_encrypted, ki_dek_wrapped
    P-->>H: Encrypted key material
    H->>K: KMS Decrypt(KEK, wrapped_DEK)
    K-->>H: Plaintext DEK
    Note over H: Decrypt Ki, OP with DEK<br/>Run MILENAGE<br/>Zero Ki, OP, DEK from memory
    H-->>E: {RAND, AUTN, XRES, CK, IK}

    E->>R: Store session {XRES, CK, IK, state=CHALLENGE_SENT} TTL=90s
    E-->>D: 401 + EAP-Request/AKA (AT_RAND, AT_AUTN, AT_MAC)

    Note over D,K: Round Trip 2: Response + Authentication

    Note over D: SIM card computes RES using Ki
    D->>A: POST /entitlement (eap_relay: AT_RES, AT_MAC)
    A->>E: Forward
    E->>R: Get session (CHALLENGE_SENT)
    R-->>E: {XRES, CK, IK}

    Note over E: Verify: RES == XRES?<br/>Verify: AT_MAC valid?<br/>Derive: MK, K_encr, K_aut, MSK

    E->>P: INSERT token (auth token)
    E->>R: SET token:{value} (cache, TTL=24h)
    E->>R: SET idempotent:{fingerprint} (cached response, TTL=90s)
    E-->>D: 200 OK + Token + Entitlement Config

    Note over D,K: Subsequent Requests: Fast Auth

    D->>A: POST /entitlement (token=<fast_auth_token>)
    A->>E: Forward
    E->>R: GET token:{value}
    R-->>E: HIT (subscriber identified)
    Note over E: Skip EAP-AKA entirely
    E->>P: Query entitlement for app_id
    P-->>E: Entitlement config
    E-->>D: 200 OK + New Token + Config
```

## Token Management: Write-Through Cache

```mermaid
graph LR
    subgraph "Token Write (after EAP-AKA success)"
        W1["Generate Token"] --> W2["INSERT Postgres<br/>(durable)"]
        W1 --> W3["SET Redis<br/>(cache, TTL)"]
        W1 --> W4["Return to Device"]
    end

    subgraph "Token Read (every request)"
        R1["Request with Token"] --> R2{"Redis<br/>GET token:value"}
        R2 -->|HIT| R3["Authenticated<br/>(sub-ms)"]
        R2 -->|MISS| R4["Query Postgres"]
        R4 -->|Found + Valid| R5["Re-populate Redis"]
        R5 --> R3
        R4 -->|Not Found / Expired| R6["401 → EAP-AKA"]
    end

    subgraph "Token Revoke"
        V1["Revoke Token"] --> V2["DEL from Redis<br/>(immediate cutoff)"]
        V1 --> V3["UPDATE Postgres<br/>consumed=true<br/>(audit trail)"]
    end
```

## Ki Security: Envelope Encryption

```mermaid
graph TB
    subgraph "Key Hierarchy"
        KEK["Cloud KMS KEK<br/>(never leaves hardware)"]
        DEK["Per-Subscriber DEK<br/>(unique per subscriber)"]
        Ki["Ki + OP<br/>(subscriber secrets)"]

        KEK -->|"Wraps"| DEK
        DEK -->|"Encrypts"| Ki
    end

    subgraph "At Rest (PostgreSQL)"
        DB_Ki["ki_encrypted<br/>(AES-256-GCM ciphertext)"]
        DB_OP["op_encrypted<br/>(AES-256-GCM ciphertext)"]
        DB_DEK["ki_dek_wrapped<br/>(KMS-wrapped DEK)"]
    end

    subgraph "At Vector Generation (Mock HSS Memory)"
        Step1["1. Read ciphertext from Postgres"]
        Step2["2. KMS unwrap DEK"]
        Step3["3. Decrypt Ki, OP"]
        Step4["4. Run MILENAGE"]
        Step5["5. ZERO Ki, OP, DEK"]
        Step6["6. Return vectors only"]

        Step1 --> Step2 --> Step3 --> Step4 --> Step5 --> Step6
    end

    Ki -.->|"Stored as"| DB_Ki
    Ki -.->|"Stored as"| DB_OP
    DEK -.->|"Stored as"| DB_DEK

    style KEK fill:#ffd93d,color:#000
    style Ki fill:#ff6b6b,color:#fff
    style Step5 fill:#ff6b6b,color:#fff
```

## Request Processing Pipeline (Fastify Hooks)

```mermaid
graph TB
    REQ["Incoming Request"] --> H1

    subgraph "onRequest Hooks"
        H1["User-Agent Parser<br/>Validate PRD-TS43 format<br/>Extract vendor, model, OS"]
        H2["Request Normalizer<br/>GET: coerce query params<br/>POST: Fastify schema validation"]
        H3["Version Check<br/>Supported entitlement_version?<br/>→ 406 if not"]
    end

    subgraph "preHandler Hook"
        H4{"Authentication"}
        H4 -->|"Token present"| H4a["Redis lookup<br/>(fast auth)"]
        H4 -->|"eap_relay present"| H4b["Continue EAP-AKA<br/>exchange"]
        H4 -->|"Neither"| H4c["Start new EAP-AKA<br/>challenge → 401"]
    end

    subgraph "Route Handler"
        H5["Entitlement Service Router<br/>Route by AppID"]
        H5 --> S1["ap2003: VoLTE/VoNR"]
        H5 --> S2["ap2004: VoWiFi"]
        H5 --> S3["ap2005: SMSoIP"]
        H5 --> S4["ap2006: ODSA Companion"]
        H5 --> S5["ap2009: ODSA Primary"]
        H5 --> S6["ap2010-2016: Others"]
    end

    subgraph "preSerialization Hook"
        H6["Response Builder<br/>XML (WAP-Provisioning)<br/>or JSON"]
    end

    H1 --> H2 --> H3 --> H4
    H4a --> H5
    H4b --> H5
    S1 --> H6
    S2 --> H6
    S3 --> H6
    S4 --> H6
    S5 --> H6
    S6 --> H6
    H6 --> RESP["HTTP Response<br/>200 / 401 / 302 / 4xx / 5xx"]
```

## Implementation Phases

```mermaid
gantt
    title Implementation Roadmap
    dateFormat X
    axisFormat %s

    section Foundation
    Phase 1: Scaffolding, Fastify, OpenTelemetry, Pino, Drizzle    :p1, 0, 1

    section Authentication
    Phase 2: Mock HSS + MILENAGE + 3GPP Test Vectors               :p2, after p1, 1
    Phase 3: EAP-AKA State Machine + Idempotency                   :p3, after p2, 1

    section Core Services
    Phase 4: Token Management + Fast Auth                           :p4, after p3, 1
    Phase 5: VoWiFi, VoLTE, SMSoIP Handlers                        :p5, after p4, 1

    section Advanced
    Phase 6: ODSA Flows (Companion + Primary)                       :p6, after p5, 1
    Phase 7: Extended Services (ap2010-2016)                        :p7, after p6, 1

    section Operations
    Phase 8: Observability (Metrics, Dashboards, Alerts)            :p8, after p7, 1
    Phase 9: Testing and Hardening                                  :p9, after p8, 1
```
