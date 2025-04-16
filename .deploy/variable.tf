variable "mcpServers" {
  type = map(object({
    command = string
    args    = optional(list(string))
    env     = optional(map(any))
  }))
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
  nullable = true
}

variable "timeoutNs" {
  type    = number
  default = 10000000000 # 10s
}

variable "llmProviderName" {
  type    = string
  default = "anthropic"
}

variable "llmApiKey" {
  type      = string
  sensitive = true
  nullable  = true
}

variable "llmBaseUrl" {
  type      = string
  sensitive = true
  default   = ""
}

variable "llmModelName" {
  type    = string
  default = ""
}

variable "slackBotToken" {
  type      = string
  sensitive = true
  nullable  = false
}

variable "slackSigninSecret" {
  type      = string
  sensitive = true
  nullable  = false
}

variable "allowedUsers" {
  type    = list(string)
  default = []
}

variable "gcpProjectID" {
  type      = string
  sensitive = true
  nullable  = false
}

variable "gcpProjectNumber" {
  type      = string
  sensitive = true
  nullable  = false
}

variable "gcpRegion" {
  type    = string
  default = "asia-northeast1"
}



