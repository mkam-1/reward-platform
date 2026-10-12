output "alb_dns_name" {
  value = aws_lb.main.dns_name
}

output "ecr_repository_urls" {
  value = { for k, r in aws_ecr_repository.services : k => r.repository_url }
}

output "dynamodb_table_name" {
  value = aws_dynamodb_table.rewards.name
}

output "rds_endpoint" {
  value = aws_db_instance.players.address
}
