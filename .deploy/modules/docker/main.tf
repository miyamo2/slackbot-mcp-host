terraform {
  required_version = ">= 1.11.0"

  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = "3.0.2"
    }
    local = {
      source  = "hashicorp/local"
      version = "2.5.2"
    }
  }
}

provider "docker" {
  registry_auth {
    address  = format("https://%s", var.host)
    username = "_json_key_base64"
    password = var.credentials
  }
}

resource "local_file" "dockerfile" {
  content = templatefile("${path.module}/Dockerfile.tpl", {
    go_installs = var.bundle.go.packages
  })
  filename = "../Dockerfile"
}

resource "local_file" "config" {
  filename = "../cmd/config.json"
  content  = jsonencode(var.config)
}

resource "docker_image" "this" {
  name         = format("%s/slackbot-mcphost:%s", var.registry, uuid())
  platform     = "linux/amd64"
  keep_locally = true
  build {
    context  = ".."
    no_cache = true
    platform = "linux/amd64"
  }
  depends_on = [
    local_file.dockerfile,
    local_file.config,
  ]
}

resource "docker_registry_image" "this" {
  name          = docker_image.this.name
  keep_remotely = true
}