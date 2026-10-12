# The one real secret in this system: the Postgres connection string
# (user+password included). Everything else — SQS queue URLs, the
# DynamoDB table name, AWS_REGION — is non-sensitive config and goes in
# the task definition's plain `environment` block instead, not here.
resource "aws_secretsmanager_secret" "database_url" {
  name = "reward-platform/database-url"
}

resource "aws_secretsmanager_secret_version" "database_url" {
  secret_id     = aws_secretsmanager_secret.database_url.id
  secret_string = "postgres://${aws_db_instance.players.username}:${var.db_password}@${aws_db_instance.players.address}:5432/${aws_db_instance.players.db_name}?sslmode=require"
}
