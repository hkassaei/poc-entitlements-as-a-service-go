output "key_ring_id" {
  value = google_kms_key_ring.entitlement_keys.id
}

output "crypto_key_id" {
  value = google_kms_crypto_key.ki_kek.id
}

output "crypto_key_name" {
  value = google_kms_crypto_key.ki_kek.name
}
