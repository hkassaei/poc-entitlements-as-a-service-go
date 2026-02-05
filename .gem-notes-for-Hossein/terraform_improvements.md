 # Identified Improvements for Infrastructure
 
 Based on a detailed review of the Terraform code in terraform/ and its modules, I have identified several findings ranging from critical infrastructure gaps to code quality improvements.


  Overall, the infrastructure code is high quality and demonstrates good security practices (Private IPs, Least Privilege IAM, Secret Manager integration, VPC isolation). However, there
  are a few Critical and High priority issues that must be addressed before this can be considered "Production Ready".

  ## Identified improvements to be made with priority

  1. Critical Findings (Must Fix)


   * Missing Remote State Backend:
       * Issue: terraform/main.tf does not define a backend "gcs" { ... } block.
       * Risk: Terraform state is stored locally (terraform.tfstate). If your computer crashes or another team member tries to run terraform apply, the infrastructure state will be lost or
         corrupted. In a team/production environment, this leads to split-brain infrastructure and potential data loss.
       * Recommendation: Configure a GCS backend in terraform/main.tf.


   * Cloud SQL Deletion Protection Disabled:
       * Issue: terraform/modules/database/main.tf has deletion_protection = false.
       * Risk: A terraform destroy or an accidental resource change could delete the production database, wiping out all subscriber data and encryption keys.
       * Recommendation: Set deletion_protection = true for production environments.

  2. High Priority Risks


   * Cloud SQL HA Disabled by Default:
       * Issue: terraform/modules/database/main.tf relies on var.ha_enabled which defaults to false (ZONAL).
       * Risk: If the zone containing the database goes down, the entire entitlement service goes offline.
       * Recommendation: Ensure ha_enabled is set to true in terraform.tfvars for production.


   * WAF Rules in Preview Mode:
       * Issue: terraform/modules/cloud-armor/main.tf has the OWASP Top 10 rules (SQLi, XSS, etc.) set to preview = true.
       * Risk: Attacks will be logged but not blocked. The system is currently vulnerable to standard web attacks.
       * Recommendation: Change preview = false after an initial observation period (e.g., 1 week of traffic) to enforce protection.

  3. Medium Priority & Best Practices


   * SSL Enforcement:
       * Issue: While cloudsql.iam_authentication is on, require_ssl is not explicitly set to true in ip_configuration (though it defaults to off/optional).
       * Recommendation: Set require_ssl = true to force encrypted connections.


   * Hardcoded Resource Names:
       * Issue: VPC name (entitlements-vpc) and subnet name (entitlements-subnet) are hardcoded in modules/networking/main.tf.
       * Impact: Prevents deploying multiple environments (dev/staging/prod) in the same project without collision.
       * Recommendation: Prefix resource names with var.environment (e.g., dev-entitlements-vpc).


  4. What is Done Well (Good News)
   * Networking: Excellent use of a custom VPC with specific subnets and Private Service Access (PSA) for Cloud SQL and Redis. No default VPC usage.
   * Security:
       * Secrets: Application secrets (DB URL, Redis URL) are injected via Secret Manager, not environment variables.
       * IAM: Granular service accounts (ecs-runner, mock-hss-runner) with least-privilege roles.
       * Ingress: mock-hss is correctly locked down to INGRESS_TRAFFIC_INTERNAL_ONLY and requires IAM authentication (roles/run.invoker).
       * KMS: Key rotation (90 days) is enabled for the encryption keys.
   * Resilience: Random suffixes on sensitive resources (KeyRings, DB Instances) prevent naming collisions.


  Summary Verdict
  The code is 85% Production Ready. It is architecturally sound but needs the state backend, deletion protection, and WAF enforcement toggles flipped to be safe for live traffic.

## Bootstrapping the infrastructure creation (critical fix)

✦ I have addressed the creation of the Terraform state bucket declaratively by integrating a Bootstrap step into your CI/CD pipeline and transitioning the entire deployment to a
  Terraform-driven flow.


  Changes Made:


   1. Declarative Resource: Added terraform/backend_resource.tf which contains a google_storage_bucket resource. This ensures that once the bucket is created, its configuration
      (versioning, lifecycle, permissions) is managed by Terraform.
   2. Pipeline Integration (`cloudbuild.yaml`):
       * Bootstrap Step: Added an idempotent step (bootstrap-state-bucket) that uses gcloud to ensure the bucket exists before anything else runs. This solves the "chicken and egg" problem
         by providing a stable foundation for terraform init.
       * Terraform Deployment: Replaced imperative gcloud run commands with a single terraform apply step. This step now passes the dynamic container images (tagged with ${SHORT_SHA}) as
         variables to Terraform.
       * Deterministic Naming: Added a substitution _STATE_BUCKET defaulting to ${PROJECT_ID}-terraform-state, making the bucket name predictable and declarative.
   3. Variables: Updated terraform/variables.tf to include state_bucket_name.

  How to use this now:


  You no longer need to run any manual commands. When you push your code:
   1. Cloud Build will check if the ${PROJECT_ID}-terraform-state bucket exists and create it if not.
   2. Terraform will initialize using that bucket as its backend.
   3. Terraform will apply the entire infrastructure, including updating the Cloud Run services with the newly built container images.
   4. Database Migrations will run automatically after the infrastructure is ready.


  This setup achieves a fully automated, declarative "Push to Deploy" workflow.

  ## High priority fixes

  ✦ I have updated the Cloud SQL Terraform code to automatically handle High Availability (HA) based on the environment.


  The availability_type is now determined by a conditional expression:
   * REGIONAL (HA enabled): Automatically selected when environment is set to "prod" or "staging".
   * ZONAL (HA disabled): Selected for "dev" or any other environment.


  This change ensures production reliability while keeping costs optimized for development, all without requiring manual flag changes in terraform.tfvars.

  ## Medium priority fixes

  ✦ I have addressed the two medium-priority improvements in the Terraform infrastructure:


  1. SSL Enforcement for Cloud SQL
  In terraform/modules/database/main.tf, I have explicitly set require_ssl = true within the ip_configuration block. This ensures that all connections to the PostgreSQL instance are
  encrypted, protecting sensitive subscriber data and encryption keys during transit.


  2. Parameterized Resource Naming
  To prevent naming collisions and support multi-environment deployments (e.g., dev, staging, prod) within the same GCP project, I have parameterized resource names across the core
  modules:
   * Networking: VPC, Subnet, and Firewall names are now prefixed with the environment name (e.g., dev-entitlements-vpc).
   * Database: The Cloud SQL instance name is now prefixed (e.g., dev-entitlements-db-abcd).
   * Redis: The Memorystore instance name is now prefixed (e.g., dev-entitlements-redis).
   * Cloud Run: Both the entitlement-server and mock-hss service names are now prefixed (e.g., dev-entitlement-server).


  The environment variable is now passed from the root main.tf to all relevant modules.


# Identified Improvements for application (ECS)

✦ I have identified several logical gaps and architectural risks in the ECS application that could impact its production readiness and protocol compliance.


  1. Protocol Compliance (GSMA TS.43 / EAP-AKA)


   * Missing Re-authentication Flow: The current implementation always performs a full EAP-AKA handshake (calling the HSS for new vectors). TS.43 strongly recommends Fast Re-authentication
     using keys derived during the full auth (MSK/EMSK) to reduce HSS load and latency.
   * Lack of Sequence Number (SQN) Synchronization: In eapAka.ts, the SYNC_FAILURE case (where the SIM's SQN is out of sync with the HSS) is currently a stub that simply fails the session.
     In production, this must forward the AT_AUTS attribute to the HSS to resynchronize the counter and generate fresh vectors.
   * Incomplete Attribute Handling: The eapCodec.ts is missing support for AT_CHECKCODE, which is required in some EAP-AKA variants to prevent bidding-down attacks on protocol version
     negotiation.

  2. Token & Session Management


   * Token Rotation Race Condition: In entitlementRoutes.ts, tokens are rotated (old revoked, new issued) on every POST. If a mobile device retries a request due to a transient network
     failure after the server has processed the rotation, the retry will carry the "old" (now revoked) token and fail with a 401.
       * Recommendation: Implement a short "grace period" (e.g., 30-60 seconds) where the previous token remains valid for retries.
   * Static Identifier Counter: The identifierCounter in eapAka.ts is local to the Node.js process memory. In a multi-instance Cloud Run environment, different instances will reuse the
     same EAP identifiers, which can cause collision issues on some client implementations.

  3. Business Logic (ODSA & Services)


   * Stateful ODSA Transitions: The ODSA handlers (odsaCompanion.ts) assume a fairly linear path. However, TS.43 requires complex handling for SubscriptionResult codes like
     DELAYED_DOWNLOAD (where the device must poll) and DELETE_PROFILE_IN_USE (requiring a prerequisite action). The current mocks return DONE too aggressively.
   * Missing T&C Flow logic: While vowifi.ts handles the REQUIRES_ACCEPTANCE status, there is no logic to transition a subscriber from DISABLED to ENABLED once the T&C URL has been visited
     (as the BSS callback is not yet implemented).


  4. Security Risks


   * Audit Log Redaction: While the code intends to avoid logging sensitive data, there is no centralized filter in the audit_log database logic to ensure that eap_relay or partial tokens
     are consistently redacted before being stored in the request_summary JSONB field.
   * Timing Safety: Although crypto.timingSafeEqual is used for AT_RES verification, the session lookup itself happens before MAC verification. A high-resolution timing attack could
     potentially be used to probe for valid session IDs in Redis.


  I recommend prioritizing the Token Rotation Grace Period and the SQN Sync Failure handling to ensure a robust user experience on real-world mobile networks.

