# Root config every env's terragrunt.hcl includes. Generates the S3
# backend block Terraform needs (versions.tf's `backend "s3"` block is
# only a placeholder shape for when Terraform runs without Terragrunt —
# Terragrunt overrides it with this, parameterized per env via `path_relative_to_include()`
# so dev/stage/prod each get their own state key without copy-pasting
# a backend block into every env).
remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    bucket         = "reward-platform-tfstate"
    key            = "${path_relative_to_include()}/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "reward-platform-tflock"
    encrypt        = true
  }
}

generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite"
  contents  = <<EOF
provider "aws" {
  region = "us-east-1"
  default_tags {
    tags = {
      Project   = "reward-platform"
      ManagedBy = "terragrunt"
    }
  }
}
EOF
}

inputs = {
  aws_region = "us-east-1"
}
