terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  backend "s3" {
    bucket       = "aws-portfolio-tfstate-786742959962"
    key          = "wildlife-gallery/terraform.tfstate"
    region       = "eu-central-1"
    use_lockfile = true
    encrypt      = true
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      project    = "aws-portfolio"
      repo       = "wildlife-gallery"
      owner      = "praveen"
      managed-by = "terraform"
    }
  }
}
