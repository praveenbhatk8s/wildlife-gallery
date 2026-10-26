data "aws_caller_identity" "current" {}

locals {
  account_id     = data.aws_caller_identity.current.account_id
  originals_name = "${var.name}-originals-${local.account_id}"
  derived_name   = "${var.name}-derived-${local.account_id}"
  function_name  = "${var.name}-process-photo"
  photo_suffixes = toset([".jpg", ".jpeg", ".JPG", ".JPEG"]) # S3 suffix filters are case-sensitive
}
