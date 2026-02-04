output "instance_name" {
  value = google_sql_database_instance.postgres.name
}

output "connection_name" {
  value = google_sql_database_instance.postgres.connection_name
}

output "private_ip" {
  value = google_sql_database_instance.postgres.private_ip_address
}

output "connection_url" {
  value     = "postgresql://ecs:${random_password.db_password.result}@${google_sql_database_instance.postgres.private_ip_address}:5432/entitlements"
  sensitive = true
}

output "db_password" {
  value     = random_password.db_password.result
  sensitive = true
}
