resource "google_artifact_registry_repository" "this" {
  project       = var.project_id
  repository_id = "slackbot-mcphost"
  format        = "DOCKER"

  docker_config {
    immutable_tags = false
  }
}

resource "google_artifact_registry_repository_iam_member" "reader" {
  repository = google_artifact_registry_repository.this.id
  role       = "roles/artifactregistry.reader"
  member     = "serviceAccount:${var.project_number}-compute@developer.gserviceaccount.com"
}

resource "google_service_account" "this" {
  account_id   = "slackbot-mcphost-gar-writer"
  display_name = "Artifact Registry Writer"
}

resource "google_artifact_registry_repository_iam_member" "writer" {
  repository = google_artifact_registry_repository.this.id
  role       = "roles/artifactregistry.writer"
  member     = google_service_account.this.member
}

resource "google_service_account_key" "this" {
  service_account_id = google_service_account.this.name
}

