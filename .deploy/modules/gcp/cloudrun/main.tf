resource "google_cloud_run_v2_service" "this" {
  name     = var.app_name
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    scaling {
      min_instance_count = 0
      max_instance_count = 1
    }
    containers {
      name  = var.app_name
      image = var.image
      resources {
        limits = {
          "cpu"    = "8"
          "memory" = "16Gi"
        }
        startup_cpu_boost = true
      }
      ports {
        container_port = 8080
      }
      startup_probe {
        initial_delay_seconds = 30
        period_seconds        = 130
        timeout_seconds       = 120
        failure_threshold     = 5
        http_get {
          path = "/health"
        }
      }
      liveness_probe {
        initial_delay_seconds = 60
        period_seconds        = 10
        timeout_seconds       = 5
        failure_threshold     = 3
        http_get {
          path = "/health"
        }
      }
    }
  }
}

resource "google_cloud_run_service_iam_binding" "this" {
  location = google_cloud_run_v2_service.this.location
  service  = google_cloud_run_v2_service.this.name
  role     = "roles/run.invoker"
  members = [
    "allUsers"
  ]
}