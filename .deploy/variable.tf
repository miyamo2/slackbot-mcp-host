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
    uv = optional(object({
      packages = list(string)
      }), {
      packages = []
    })
    bun = optional(object({
      packages = list(string)
      }), {
      packages = []
    })
    npm = optional(object({
      packages = list(string)
      }), {
      packages = []
    })
  })
  default = {
    go = {
      packages = []
    }
    uv = {
      packages = []
    }
    bun = {
      packages = []
    }
    npm = {
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

variable "rateLimit" {
  type = object({
    enable    = optional(bool, false)
    limit     = optional(number, 20)
    burst     = optional(number)
    expiresIn = optional(number, 300)
  })
  nullable = true
}



