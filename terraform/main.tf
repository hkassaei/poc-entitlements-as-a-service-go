terraform {
  required_version = ">= 1.5"

  backend "gcs" {
    bucket = "REPLACE_WITH_YOUR_TERRAFORM_STATE_BUCKET"
    prefix = "terraform/state"
  }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# Enable required GCP APIs
resource "google_project_service" "apis" {
  for_each = toset([
    "run.googleapis.com",
    "sqladmin.googleapis.com",
    "redis.googleapis.com",
    "cloudkms.googleapis.com",
    "secretmanager.googleapis.com",
    "artifactregistry.googleapis.com",
    "cloudbuild.googleapis.com",
    "compute.googleapis.com",
    "servicenetworking.googleapis.com",
    "cloudtrace.googleapis.com",
  ])

  service            = each.key
  disable_on_destroy = false
}

module "networking" {
  source      = "./modules/networking"
  project_id  = var.project_id
  region      = var.region
  environment = var.environment

  depends_on = [google_project_service.apis]
}

module "iam" {
  source     = "./modules/iam"
  project_id = var.project_id
}

# Module-level depends_on ensures VPC peering is fully established
# before Cloud SQL tries to create a private-IP instance.
module "database" {
  source                = "./modules/database"
  project_id            = var.project_id
  region                = var.region
  environment           = var.environment
  network_id            = module.networking.network_id
  private_ip_range_name = module.networking.private_ip_range_name
  tier                  = var.db_tier
  ha_enabled            = var.db_ha_enabled

  depends_on = [google_project_service.apis, module.networking]
}

module "redis" {
  source      = "./modules/redis"
  project_id  = var.project_id
  region      = var.region
  environment = var.environment
  network_id  = module.networking.network_id

  depends_on = [google_project_service.apis, module.networking]
}

module "kms" {
  source                      = "./modules/kms"
  project_id                  = var.project_id
  region                      = var.region
  mock_hss_service_account    = module.iam.mock_hss_service_account_email
  cloud_build_service_account = module.iam.cloud_build_service_account_email

  depends_on = [google_project_service.apis]
}

module "secrets" {
  source                         = "./modules/secrets"
  project_id                     = var.project_id
  region                         = var.region
  environment                    = var.environment
  database_url                   = module.database.connection_url
  redis_url                      = module.redis.connection_url
  ecs_service_account_email      = module.iam.ecs_service_account_email
  mock_hss_service_account_email = module.iam.mock_hss_service_account_email

  depends_on = [google_project_service.apis]
}

module "artifact_registry" {
  source     = "./modules/artifact-registry"
  project_id = var.project_id
  region     = var.region

  depends_on = [google_project_service.apis]
}

module "cloud_run" {
  source                         = "./modules/cloud-run"
  project_id                     = var.project_id
  region                         = var.region
  environment                    = var.environment
  vpc_network                    = module.networking.network_name
  vpc_subnetwork                 = module.networking.subnet_name
  cloudsql_connection_name       = module.database.connection_name
  ecs_image                      = var.ecs_image
  mock_hss_image                 = var.mock_hss_image
  ecs_service_account_email      = module.iam.ecs_service_account_email
  mock_hss_service_account_email = module.iam.mock_hss_service_account_email
  database_url_secret_id         = module.secrets.database_url_secret_id
  redis_url_secret_id            = module.secrets.redis_url_secret_id
  operator_mcc                   = var.operator_mcc
  operator_mnc                   = var.operator_mnc
  operator_name                  = var.operator_name

  depends_on = [
    google_project_service.apis,
    module.secrets,
    module.iam,
    module.networking,
    module.database,
  ]
}

module "cloud_armor" {
  source = "./modules/cloud-armor"

  depends_on = [google_project_service.apis]
}

module "load_balancer" {
  source                 = "./modules/load-balancer"
  project_id             = var.project_id
  region                 = var.region
  cloud_run_service_name = module.cloud_run.ecs_service_name
  security_policy_id     = module.cloud_armor.policy_id
  domain                 = var.domain

  # No explicit depends_on — variable references to cloud_run and cloud_armor
  # outputs create implicit ordering. Module-level depends_on would cause a
  # dependency cycle through the networking module's global address resources.
}
