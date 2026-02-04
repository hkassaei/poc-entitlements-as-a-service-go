terraform {
  required_version = ">= 1.5"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
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
    "vpcaccess.googleapis.com",
    "servicenetworking.googleapis.com",
    "cloudtrace.googleapis.com",
  ])

  service            = each.key
  disable_on_destroy = false
}

module "networking" {
  source     = "./modules/networking"
  project_id = var.project_id
  region     = var.region

  depends_on = [google_project_service.apis]
}

module "iam" {
  source     = "./modules/iam"
  project_id = var.project_id
}

module "database" {
  source              = "./modules/database"
  project_id          = var.project_id
  region              = var.region
  network_id          = module.networking.network_id
  private_ip_range_name = module.networking.private_ip_range_name
  tier                = var.db_tier
  ha_enabled          = var.db_ha_enabled

  depends_on = [google_project_service.apis, module.networking]
}

module "redis" {
  source     = "./modules/redis"
  project_id = var.project_id
  region     = var.region
  network_id = module.networking.network_id

  depends_on = [google_project_service.apis, module.networking]
}

module "kms" {
  source                   = "./modules/kms"
  project_id               = var.project_id
  region                   = var.region
  mock_hss_service_account = module.iam.mock_hss_service_account_email

  depends_on = [google_project_service.apis]
}

module "secrets" {
  source       = "./modules/secrets"
  project_id   = var.project_id
  region       = var.region
  database_url = module.database.connection_url
  redis_url    = module.redis.connection_url
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
  vpc_connector_id               = module.networking.vpc_connector_id
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
  ]
}

module "cloud_armor" {
  source = "./modules/cloud-armor"

  depends_on = [google_project_service.apis]
}
