# The one guardrail that matters most for a free-tier account: a
# budget alarm that emails before costs run away unnoticed, independent
# of whether any individual resource is "free tier eligible" on paper.
resource "aws_sns_topic" "alarms" {
  name = "reward-platform-alarms"
}

resource "aws_sns_topic_subscription" "alarms_email" {
  topic_arn = aws_sns_topic.alarms.arn
  protocol  = "email"
  endpoint  = var.alarm_email
}

resource "aws_budgets_budget" "monthly" {
  name         = "reward-platform-monthly"
  budget_type  = "COST"
  limit_amount = "10"
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.alarm_email]
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = [var.alarm_email]
  }
}

# Per-service CPU alarm — not for autoscaling (desired_count is fixed at
# 1 for cost control, see ecs.tf), just a signal that something is
# stuck in a retry loop or otherwise working harder than expected.
resource "aws_cloudwatch_metric_alarm" "cpu_high" {
  for_each = local.services

  alarm_name          = "reward-platform-${each.key}-cpu-high"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 3
  metric_name         = "CPUUtilization"
  namespace           = "AWS/ECS"
  period              = 60
  statistic           = "Average"
  threshold           = 80
  alarm_actions       = [aws_sns_topic.alarms.arn]

  dimensions = {
    ClusterName = aws_ecs_cluster.main.name
    ServiceName = each.key
  }
}
