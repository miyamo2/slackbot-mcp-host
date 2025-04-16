terraform {
  required_version = ">= 1.11.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
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

provider "google" {
  project = var.gcpProjectID
  region  = var.gcpRegion
}

module "gar" {
  source         = "./modules/gcp/gar"
  project_id     = var.gcpProjectID
  project_number = var.gcpProjectNumber
}

provider "docker" {
  registry_auth {
    address  = format("https://%s", module.gar.host)
    username = "_json_key_base64"
    password = module.gar.credentials
  }
}

module "docker" {
  source      = "./modules/docker"
  host        = module.gar.host
  credentials = module.gar.credentials
  registry    = module.gar.registry
  bundle     = var.bundle
  config = {
    mcpServers        = var.mcpServers
    timeoutNs         = var.timeoutNs
    llmProviderName   = var.llmProviderName
    llmApiKey         = sensitive(var.llmApiKey)
    llmBaseUrl        = sensitive(var.llmBaseUrl)
    llmModelName      = var.llmModelName
    slackBotToken     = sensitive(var.slackBotToken)
    slackSigninSecret = sensitive(var.slackSigninSecret)
    allowedUsers      = sensitive(var.allowedUsers)
    gcpProjectID      = var.gcpProjectID
  }
}

module "cloudrun" {
  source = "./modules/gcp/cloudrun"
  region = var.gcpRegion
  image  = module.docker.image
}

