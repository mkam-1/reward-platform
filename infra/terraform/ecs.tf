resource "aws_ecs_cluster" "main" {
  name = "reward-platform"

  setting {
    name  = "containerInsights"
    value = "disabled" # Container Insights bills per-metric; skip it for a learning deployment
  }
}

resource "aws_cloudwatch_log_group" "services" {
  for_each          = toset(["player-service", "game-service", "rewards-service"])
  name              = "/ecs/reward-platform/${each.key}"
  retention_in_days = 7
}

locals {
  services = {
    player-service = {
      task_role   = aws_iam_role.player_service_task.arn
      cpu         = 256
      memory      = 512
      port        = 8080
      extra_ports = [9090]
      environment = [
        { name = "PORT", value = "8080" },
        { name = "GRPC_PORT", value = "9090" },
        { name = "SQS_ENDPOINT", value = "" },
        { name = "SESSION_COMPLETED_QUEUE", value = aws_sqs_queue.session_completed.url },
        { name = "POINTS_AWARDED_QUEUE", value = aws_sqs_queue.points_awarded.url },
        { name = "AWS_REGION", value = var.aws_region },
      ]
      secrets = [
        { name = "DATABASE_URL", valueFrom = aws_secretsmanager_secret.database_url.arn },
      ]
    }
    game-service = {
      task_role   = aws_iam_role.game_service_task.arn
      cpu         = 256
      memory      = 512
      port        = 8080
      extra_ports = []
      environment = [
        { name = "PORT", value = "8080" },
        { name = "SQS_ENDPOINT", value = "" },
        { name = "SESSION_COMPLETED_QUEUE", value = aws_sqs_queue.session_completed.url },
        { name = "AWS_REGION", value = var.aws_region },
      ]
      secrets = []
    }
    rewards-service = {
      task_role   = aws_iam_role.rewards_service_task.arn
      cpu         = 256
      memory      = 512
      port        = 8080
      extra_ports = []
      environment = [
        { name = "PORT", value = "8080" },
        { name = "DYNAMODB_ENDPOINT", value = "" },
        { name = "DYNAMODB_TABLE", value = aws_dynamodb_table.rewards.name },
        { name = "PLAYER_SERVICE_GRPC_ADDR", value = "player-service.reward-platform.internal:9090" },
        { name = "AWS_REGION", value = var.aws_region },
      ]
      secrets = []
    }
  }
}

resource "aws_ecs_task_definition" "services" {
  for_each = local.services

  family                   = "reward-platform-${each.key}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = each.value.cpu
  memory                   = each.value.memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = each.value.task_role

  container_definitions = jsonencode([{
    name      = each.key
    image     = "${aws_ecr_repository.services[each.key].repository_url}:latest"
    essential = true
    portMappings = concat(
      [{ containerPort = each.value.port, protocol = "tcp" }],
      [for p in each.value.extra_ports : { containerPort = p, protocol = "tcp" }],
    )
    environment = each.value.environment
    secrets     = each.value.secrets
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.services[each.key].name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = each.key
      }
    }
  }])
}

# Cloud Map: gives rewards-service a stable DNS name for player-service
# (player-service.reward-platform.internal) instead of a hardcoded,
# ever-changing task IP — the ECS-native alternative to a k8s Service.
resource "aws_service_discovery_private_dns_namespace" "main" {
  name = "reward-platform.internal"
  vpc  = data.aws_vpc.default.id
}

resource "aws_service_discovery_service" "player_service" {
  name = "player-service"
  dns_config {
    namespace_id = aws_service_discovery_private_dns_namespace.main.id
    dns_records {
      type = "A"
      ttl  = 10
    }
  }
}

resource "aws_ecs_service" "services" {
  for_each = local.services

  name            = each.key
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.services[each.key].arn
  desired_count   = 1 # single instance per service — cost control, not an HA claim
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = data.aws_subnets.default.ids
    security_groups  = [aws_security_group.ecs_tasks.id]
    assign_public_ip = true # no NAT gateway (see network.tf) — tasks need a public IP to reach ECR/CloudWatch/DynamoDB/SQS
  }

  dynamic "service_registries" {
    for_each = each.key == "player-service" ? [1] : []
    content {
      registry_arn = aws_service_discovery_service.player_service.arn
    }
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.services[each.key].arn
    container_name   = each.key
    container_port   = each.value.port
  }

  depends_on = [aws_lb_listener.http]
}
