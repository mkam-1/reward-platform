# Two kinds of role, the distinction ECS Fargate draws explicitly:
#   - execution role: what the ECS *agent* needs before your code even
#     starts (pull the image from ECR, fetch the secret, write to
#     CloudWatch Logs).
#   - task role: what your *application code* is allowed to do once
#     running (talk to DynamoDB, SQS) — assumed via the container's own
#     credentials, no access key ever stored anywhere.
data "aws_iam_policy_document" "ecs_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "reward-platform-ecs-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "execution_secrets" {
  name = "read-database-secret"
  role = aws_iam_role.execution.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = [aws_secretsmanager_secret.database_url.arn]
    }]
  })
}

# player-service's task role: the ledger lives in Postgres (reached
# over the network, no IAM involved) and it publishes/consumes via SQS.
resource "aws_iam_role" "player_service_task" {
  name               = "reward-platform-player-service-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role_policy" "player_service_task" {
  name = "sqs-access"
  role = aws_iam_role.player_service_task.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = ["sqs:SendMessage", "sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes", "sqs:GetQueueUrl"]
      Resource = [
        aws_sqs_queue.session_completed.arn,
        aws_sqs_queue.points_awarded.arn,
      ]
    }]
  })
}

# game-service's task role: only publishes SessionCompleted, never reads.
resource "aws_iam_role" "game_service_task" {
  name               = "reward-platform-game-service-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role_policy" "game_service_task" {
  name = "sqs-publish-only"
  role = aws_iam_role.game_service_task.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["sqs:SendMessage", "sqs:GetQueueUrl"]
      Resource = [aws_sqs_queue.session_completed.arn]
    }]
  })
}

# rewards-service's task role: owns the DynamoDB table and calls
# player-service over gRPC (no IAM permission needed for that — it's a
# plain network call, authorized by the security group instead).
resource "aws_iam_role" "rewards_service_task" {
  name               = "reward-platform-rewards-service-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role_policy" "rewards_service_task" {
  name = "dynamodb-access"
  role = aws_iam_role.rewards_service_task.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:UpdateItem",
        "dynamodb:Query", "dynamodb:Scan", "dynamodb:DescribeTable",
      ]
      Resource = [
        aws_dynamodb_table.rewards.arn,
        "${aws_dynamodb_table.rewards.arn}/index/*",
      ]
    }]
  })
}
