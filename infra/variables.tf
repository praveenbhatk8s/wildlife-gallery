variable "region" {
  type    = string
  default = "eu-central-1"
}

variable "name" {
  description = "Prefix for all resource names."
  type        = string
  default     = "wildlife-gallery"
}

variable "alert_email" {
  description = "Email for failed-processing alerts (confirm the SNS subscription email after the first apply)."
  type        = string
}

variable "lambda_zip" {
  description = "Path to the built Lambda package (make build)."
  type        = string
  default     = "../dist/process-photo.zip"
}

variable "allow_destroy_originals" {
  description = "Set true and apply ONCE before destroying, to let Terraform delete the originals bucket with its photos."
  type        = bool
  default     = false
}
