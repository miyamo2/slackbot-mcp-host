variable "host" {
  type      = string
  default   = "localhost"
  sensitive = true
  nullable  = false
}

variable "credentials" {
  type      = string
  sensitive = true
  nullable  = false
}

variable "registry" {
  type      = string
  sensitive = true
  nullable  = false
}

variable "bundle" {
  type = object({
    go = optional(object({
      packages = list(string)
      }), {
      packages = []
    })
  })
  default = {
    go = {
      packages = []
    }
  }
}

variable "config" {
  type = object({
    mcpServers = map(object({
      command = string
      args    = optional(list(string))
      env     = optional(map(any))
    }))
    timeoutNs         = number
    llmProviderName   = string
    llmApiKey         = string
    llmBaseUrl        = string
    llmModelName      = string
    slackBotToken     = string
    slackSigninSecret = string
    allowedUsers      = list(string)
    gcpProjectID      = string
  })
  sensitive = true
}