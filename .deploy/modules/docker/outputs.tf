output "image" {
  value     = docker_registry_image.this.name
  sensitive = true
}