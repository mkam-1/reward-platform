# Uses the account's default VPC/subnets instead of provisioning a new
# one: a dedicated VPC with private subnets + a NAT gateway is the
# right call for a real production workload, but a NAT gateway alone
# costs real money per hour regardless of traffic — exactly the kind
# of always-on cost this project's free-tier guardrail (see README)
# rules out for a learning deployment. Fargate tasks instead run in the
# default VPC's public subnets with public IPs, locked down by security
# groups to only the ports they need from only the sources that need
# them — not "no NAT" taken as "no isolation."
data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

resource "aws_security_group" "alb" {
  name        = "reward-platform-alb"
  description = "Internet-facing ALB: allow inbound HTTP from anywhere, all outbound."
  vpc_id      = data.aws_vpc.default.id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "ecs_tasks" {
  name        = "reward-platform-ecs-tasks"
  description = "ECS tasks: allow inbound only from the ALB, plus service-to-service gRPC (player-service:9090)."
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description     = "ALB -> tasks (HTTP)"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  ingress {
    description = "rewards-service -> player-service (gRPC), same SG on both sides"
    from_port   = 9090
    to_port     = 9090
    protocol    = "tcp"
    self        = true
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "rds" {
  name        = "reward-platform-rds"
  description = "Postgres: allow inbound 5432 only from ECS tasks."
  vpc_id      = data.aws_vpc.default.id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_tasks.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "opensearch" {
  name        = "reward-platform-opensearch"
  description = "OpenSearch (log search — see logsink/ and ADR-0009): allow inbound 443 only from ECS tasks."
  vpc_id      = data.aws_vpc.default.id

  ingress {
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_tasks.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
