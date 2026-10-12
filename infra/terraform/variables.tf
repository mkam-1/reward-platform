variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "environment" {
  type    = string
  default = "dev"
}

variable "db_password" {
  description = "Postgres master password. Passed via TF_VAR_db_password or a .tfvars file that's gitignored — never committed."
  type        = string
  sensitive   = true
}

variable "alarm_email" {
  description = "Email address the budget alarm and CloudWatch alarms notify."
  type        = string
}
