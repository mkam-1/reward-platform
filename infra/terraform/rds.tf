# db.t3.micro + 20GB gp2: both inside the RDS free tier (750 hrs/month,
# 12 months). Single-AZ (no standby replica) — the free tier doesn't
# cover Multi-AZ, and this is a learning deployment, not a production
# SLA commitment.
resource "aws_db_subnet_group" "main" {
  name       = "reward-platform"
  subnet_ids = data.aws_subnets.default.ids
}

resource "aws_db_instance" "players" {
  identifier     = "reward-platform-players"
  engine         = "postgres"
  engine_version = "16"
  instance_class = "db.t3.micro"

  allocated_storage = 20
  storage_type      = "gp2"

  db_name  = "reward_platform"
  username = "reward"
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  publicly_accessible    = false

  skip_final_snapshot     = true # dev environment — no need to keep a snapshot around after destroy
  multi_az                = false
  backup_retention_period = 1
}
