# KeyRings are immutable in GCP — they cannot be deleted, ever.
# The random suffix ensures a fresh name on every terraform apply cycle,
# avoiding "already exists" errors after destroy+recreate in dev/staging.
resource "random_id" "keyring_suffix" {
  byte_length = 4
}

resource "google_kms_key_ring" "entitlement_keys" {
  name     = "entitlement-keys-${random_id.keyring_suffix.hex}"
  project  = var.project_id
  location = var.region
}

resource "google_kms_crypto_key" "ki_kek" {
  name     = "ki-kek"
  key_ring = google_kms_key_ring.entitlement_keys.id
  purpose  = "ENCRYPT_DECRYPT"

  rotation_period = "7776000s" # 90 days
}

# mock-hss decrypts Ki at runtime to compute EAP-AKA vectors
resource "google_kms_crypto_key_iam_member" "mock_hss_decrypter" {
  crypto_key_id = google_kms_crypto_key.ki_kek.id
  role          = "roles/cloudkms.cryptoKeyDecrypter"
  member        = "serviceAccount:${var.mock_hss_service_account}"
}

# Cloud Build encrypts Ki values during the seed/migration step
resource "google_kms_crypto_key_iam_member" "cloud_build_encrypter" {
  crypto_key_id = google_kms_crypto_key.ki_kek.id
  role          = "roles/cloudkms.cryptoKeyEncrypter"
  member        = "serviceAccount:${var.cloud_build_service_account}"
}
