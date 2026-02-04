resource "google_kms_key_ring" "entitlement_keys" {
  name     = "entitlement-keys"
  project  = var.project_id
  location = var.region
}

resource "google_kms_crypto_key" "ki_kek" {
  name     = "ki-kek"
  key_ring = google_kms_key_ring.entitlement_keys.id
  purpose  = "ENCRYPT_DECRYPT"

  rotation_period = "7776000s" # 90 days

  lifecycle {
    prevent_destroy = true
  }
}

resource "google_kms_crypto_key_iam_member" "mock_hss_decrypter" {
  crypto_key_id = google_kms_crypto_key.ki_kek.id
  role          = "roles/cloudkms.cryptoKeyDecrypter"
  member        = "serviceAccount:${var.mock_hss_service_account}"
}
