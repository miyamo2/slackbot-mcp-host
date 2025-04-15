output "credentials" {
  value     = google_service_account_key.this.private_key
  sensitive = true
}

output "host" {
  value     = format("%s-docker.pkg.dev", google_artifact_registry_repository.this.location)
  sensitive = true
}

output "registry" {
  value     = format("%s-docker.pkg.dev/%s/%s", google_artifact_registry_repository.this.location, var.project_id, google_artifact_registry_repository.this.name)
  sensitive = true
}
