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
                ECS["ECS (Go)<br/>chi Router + slog<br/>EAP-AKA State Machine<br/>Token Management<br/>12 Service Handlers"]
            end

            subgraph HSS_Service["Cloud Run: mock-hss (Internal-Only Ingress)"]
                HSS["Mock HSS (Go)<br/>MILENAGE Algorithm<br/>Envelope Encryption<br/>POST /vectors"]
            end

            Redis["Memorystore<br/>(Redis)<br/>EAP-AKA Sessions (90s TTL)<br/>Token Cache<br/>Idempotency Cache"]
            CloudSQL["Cloud SQL<br/>(PostgreSQL)<br/>Subscribers, Tokens,<br/>Entitlements, Audit Log,<br/>Encrypted Ki/OP"]
            KMS["Cloud KMS<br/>KEK (Key Encryption Key)<br/>Never leaves hardware"]
        end

        SecretMgr["Secret Manager<br/>DATABASE_URL, REDIS_URL"]
        Logging["Cloud Logging<br/>(slog JSON auto-ingested)"]
        Trace["Cloud Trace<br/>(OpenTelemetry)"]
        Monitoring["Cloud Monitoring<br/>Metrics + Dashboards + Alerts"]
        Artifact["Artifact Registry<br/>(Container Images)"]
        Build["Cloud Build<br/>(CI/CD Pipeline)"]
    end

    Device -->|HTTPS| Armor
    Armor --> LB
    LB -->|HTTP :8443| ECS

    ECS -->|"Token R/W<br/>Session R/W<br/>Idempotency Cache"| Redis
    ECS -->|"Token CRUD<br/>Entitlement Lookup<br/>Audit Log"| CloudSQL
    ECS -->|"POST /vectors<br/>{imsi}<br/>(VPC Internal)"| HSS

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

    D->>A: POST /entitlement {imsi, app, terminal_id}
    A->>E: Forward (after DDoS/WAF check)

    E->>H: POST /vectors {imsi}
    H->>P: Read ki_encrypted, op_encrypted, ki_dek_wrapped
    P-->>H: Encrypted key material
    H->>K: KMS Decrypt(KEK, wrapped_DEK)
    K-->>H: Plaintext DEK
    Note over H: Decrypt Ki, OP with DEK<br/>Run MILENAGE<br/>Zero Ki, OP, DEK from memory
    H-->>E: {RAND, AUTN, XRES, CK, IK}

    Note over E: Derive MK, K_encr, K_aut, MSK, EMSK<br/>Build EAP-Request/AKA-Challenge
    E->>R: Store session {XRES, CK, IK, keys, state=CHALLENGE_SENT} TTL=90s
    E-->>D: 401 + EAP-Request/AKA (AT_RAND, AT_AUTN, AT_MAC)

    Note over D,K: Round Trip 2: Response + Authentication

    Note over D: SIM card computes RES using Ki
    D->>A: POST /entitlement {eap_relay: AT_RES + AT_MAC}
    A->>E: Forward
    E->>R: Get session (CHALLENGE_SENT)
    R-->>E: {XRES, keys}

    Note over E: Verify: AT_MAC valid? (HMAC-SHA1)<br/>Verify: AT_RES == XRES? (constant-time)<br/>Issue re-auth identity

    E->>R: Store re-auth state (48h TTL)
    E->>R: SET idempotent:{fingerprint} (cached response, TTL=90s)
    E->>P: Query entitlements for app_id
    P-->>E: Entitlement config
    E-->>D: 200 OK + EAP-Success + Entitlement Config

    Note over D,K: Subsequent Requests: Fast Re-auth (RFC 4187 §5.1)

    D->>A: POST /entitlement {token=<reauth_id>}
    A->>E: Forward
    E->>R: GET reauth:{id}
    R-->>E: HIT (re-auth state found)
    Note over E: Build EAP-Request/AKA-Reauthentication<br/>Encrypted inner attributes (AES-128-CBC)
    E-->>D: 401 + Re-auth Challenge
    D->>A: POST /entitlement {eap_relay: re-auth response}
    A->>E: Forward
    Note over E: Verify counter + MAC<br/>Derive new keys<br/>Rotate re-auth identity
    E->>P: Query entitlements for app_id
    P-->>E: Entitlement config
    E-->>D: 200 OK + Entitlement Config
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
        Step5["5. defer crypto.ZeroSlice(ki, op, dek)"]
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

## Request Processing Pipeline (chi Middleware)

```mermaid
graph TB
    REQ["Incoming Request"] --> H1

    subgraph "Middleware Stack (r.Use)"
        H1["RealIP<br/>Extract client IP from<br/>X-Forwarded-For / X-Real-IP"]
        H2["RecoverMiddleware<br/>Catch panics → 500<br/>Log stack trace"]
        H3["RequestLogger<br/>Log method, path, status,<br/>duration for every request"]
        H4["UserAgentParser<br/>Validate PRD-TS43 format<br/>Extract vendor, model, OS"]
        H5["VersionCheck<br/>Supported entitlement_version?<br/>→ 406 if not"]
    end

    subgraph "Route Handler (POST /entitlement)"
        H6{"4-Path Router"}
        H6 -->|"eap_relay present"| P1["Path 1: EAP Response<br/>Try re-auth session first,<br/>then full auth RT2"]
        H6 -->|"token present"| P2["Path 2: Token<br/>Try re-auth RT1,<br/>then ODSA temp token"]
        H6 -->|"neither"| P3["Path 3: Initial<br/>EAP-AKA RT1<br/>(requires IMSI)"]
    end

    subgraph "Service Router"
        SR["ResponseBuilder<br/>Route by AppID"]
        SR --> S1["ap2003: VoLTE/VoNR"]
        SR --> S2["ap2004: VoWiFi"]
        SR --> S3["ap2005: SMSoIP"]
        SR --> S4["ap2006: ODSA Companion"]
        SR --> S5["ap2009: ODSA Primary"]
        SR --> S6["ap2010-2016: Others"]
    end

    subgraph "Response Format"
        RF["Response Builder<br/>XML (WAP-Provisioning)<br/>or JSON"]
    end

    H1 --> H2 --> H3 --> H4 --> H5 --> H6
    P1 --> SR
    P2 --> SR
    S1 --> RF
    S2 --> RF
    S3 --> RF
    S4 --> RF
    S5 --> RF
    S6 --> RF
    RF --> RESP["HTTP Response<br/>200 / 401 / 4xx / 5xx"]
```

## Project Structure

```
cmd/
  ecs/main.go                    # ECS server entry point
  mock-hss/main.go               # Mock HSS entry point
internal/
  config/                        # Config struct + EAP-AKA constants
  crypto/                        # MILENAGE, AES-256-GCM envelope, KMS
  eapaka/                        # EAP-AKA protocol stack
    codec.go                     # EAP packet binary encode/decode
    keys.go                      # MK derivation, FIPS 186-2 PRF
    encryption.go                # AES-128-CBC for AT_ENCR_DATA
    orchestrator.go              # Full auth state machine (RT1/RT2)
    reauth.go                    # Fast re-auth (RFC 4187 §5.1)
    session.go                   # Redis EAP session CRUD (90s TTL)
    reauth_session.go            # Redis re-auth session (90s TTL)
    reauth_store.go              # Redis re-auth state (48h TTL)
    idempotency.go               # Response replay cache (90s TTL)
    vectors.go                   # HTTP client to mock-hss
  db/                            # pgx/v5 pool, queries, models, migrate, seed
  token/                         # Token generation/validation, Redis cache
  protocol/                      # TS.43 JSON + XML builders, status codes
  services/                      # 12 telecom service handlers
  server/                        # chi router, middleware, routes, audit
sql/schema.sql                   # DDL for 5 tables
Dockerfile.ecs                   # Multi-stage → distroless
Dockerfile.mock-hss
docker-compose.yml
Makefile
```

## CI/CD Pipeline Architecture

```mermaid
graph TD
    classDef github fill:#24292e,stroke:#fff,color:#fff;
    classDef gcp fill:#4285F4,stroke:#fff,color:#fff;
    classDef security fill:#d32f2f,stroke:#fff,color:#fff;
    classDef gate fill:#f9a825,stroke:#333,color:#000;

    subgraph Github_Actions ["GitHub Actions (CI & Security Gates)"]
        direction TB
        Start((Developer Commit)) --> PR[Create Pull Request]

        subgraph Quality_Checks ["Quality & Build Gates"]
            Build[go build ./...]
            Vet[go vet ./...]
            Fmt[gofmt -l .]
            Lint[golangci-lint]
        end

        subgraph Test_Gates ["Test Gates"]
            Unit[go test ./... -race]
            Vuln[govulncheck ./...]
        end

        subgraph Security_Gates ["Security Gates"]
            CodeQL[CodeQL: Go SAST]
            Gitleaks[Gitleaks: Secret Scanning]
            Checkov[Checkov: IaC Scanning]
            Dependabot[Dependabot: Go Module Deps]
        end

        subgraph Infra_Gates ["Infrastructure Gates"]
            TfFmt[terraform fmt -check]
            TfValidate[terraform validate]
        end

        PR --> Quality_Checks
        Quality_Checks --> Test_Gates
        PR --> Security_Gates
        PR --> Infra_Gates
    end

    Test_Gates -->|All Checks Pass| Merge{Merge to Main}:::gate
    Security_Gates --> Merge
    Infra_Gates --> Merge

    subgraph GCP_Cloud_Build ["Google Cloud Build (Trusted Artifacts)"]
        Merge --> DockerBuild[Docker Build: ECS & Mock-HSS<br/>Multi-stage → Distroless]
        DockerBuild --> Trivy[Trivy: Container Vulnerability Scan]
        Trivy --> Sign[Binary Auth: Image Signing]
    end

    subgraph GCP_Cloud_Deploy ["Google Cloud Deploy (Controlled Rollout)"]
        Sign --> DeployDev[Deploy to Dev: Cloud Run]
        DeployDev --> Smoke[Smoke / Health Tests]
        Smoke --> Approval{MANUAL APPROVAL}:::gate
        Approval -->|Approved| BinAuth[Verify Binary Authorization]
        BinAuth --> DeployProd[Deploy to Production: Cloud Run]
    end

    class PR,Build,Vet,Fmt,Lint,Unit,Vuln,CodeQL,Gitleaks,Checkov,Dependabot,TfFmt,TfValidate github;
    class DockerBuild,Trivy,Sign,DeployDev,Smoke,BinAuth,DeployProd gcp;
    class CodeQL,Gitleaks,Checkov,Dependabot,Trivy,BinAuth security;
```
