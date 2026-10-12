include "root" {
  path = find_in_parent_folders()
}

terraform {
  source = "../../terraform"
}

inputs = {
  environment = "dev"
  # db_password and alarm_email are deliberately NOT set here — pass
  # them via TF_VAR_db_password / TF_VAR_alarm_email (or a gitignored
  # dev.auto.tfvars), the same as running the plain terraform module
  # directly. A real secret never belongs in a committed .hcl file.
}
