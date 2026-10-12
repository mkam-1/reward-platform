# Amazon OpenSearch Service — AWS's managed, Elasticsearch-API-compatible
# offering (the JD names "ElasticSearch" specifically; OpenSearch is the
# fork AWS has shipped under that managed-service umbrella since 2021,
# and logsink/ talks to it over the same Elasticsearch client/API this
# whole stack is built against). See ADR-0009 for the local-dev
# (docker-compose/k8s run plain Elasticsearch directly) vs real-AWS
# (OpenSearch) split and why that's not a contradiction.
#
# t3.small.search, single node, 10GB: the smallest configuration AWS
# allows, consistent with every other cost-conscious choice in this
# file (RDS db.t3.micro, DynamoDB on-demand, Fargate 256/512). Not
# free-tier eligible the way RDS/DynamoDB are — OpenSearch's free tier
# only applies to its very first 12 months on a brand-new account —
# so this is the one resource in the whole stack worth a second look
# before `apply`, cost-wise.
resource "aws_opensearch_domain" "logs" {
  domain_name    = "reward-platform-logs"
  engine_version = "OpenSearch_2.15"

  cluster_config {
    instance_type  = "t3.small.search"
    instance_count = 1
  }

  ebs_options {
    ebs_enabled = true
    volume_size = 10
    volume_type = "gp3"
  }

  vpc_options {
    subnet_ids         = [data.aws_subnets.default.ids[0]] # single node -> single subnet; multi-AZ would need one subnet per AZ
    security_group_ids = [aws_security_group.opensearch.id]
  }

  # Network access is already the real control (the security group
  # above only admits traffic from ECS tasks' own SG) — this resource
  # policy stays permissive on top of that rather than layering IAM
  # SigV4 request signing into logsink's client, which the standard
  # Elasticsearch-API client this project uses doesn't speak natively.
  access_policies = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { AWS = "*" }
      Action    = "es:*"
      Resource  = "arn:aws:es:${var.aws_region}:*:domain/reward-platform-logs/*"
    }]
  })

  encrypt_at_rest {
    enabled = true
  }
  node_to_node_encryption {
    enabled = true
  }
  domain_endpoint_options {
    enforce_https = true
  }
}
