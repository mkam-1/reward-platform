# Every queue gets a dead-letter queue: without one, a message whose
# handler keeps failing (a malformed event, a bug that panics on one
# specific payload) gets redelivered forever — mq.Client.Consume's own
# doc comment flags this as a known gap, closed here on the
# infrastructure side rather than in application code, since a redrive
# policy is exactly that: AWS-side queue configuration, not something
# the consumer loop needs to know about. maxReceiveCount=5 means a
# message that fails its handler 5 times moves to the DLQ instead of
# looping — visible there for manual inspection/replay instead of
# silently consuming consumer capacity forever.
resource "aws_sqs_queue" "session_completed_dlq" {
  name                      = "session-completed-dlq"
  message_retention_seconds = 1209600 # 14 days — longer than the live queue's, so there's time to notice and replay
}

resource "aws_sqs_queue" "session_completed" {
  name                      = "session-completed"
  message_retention_seconds = 345600 # 4 days
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.session_completed_dlq.arn
    maxReceiveCount     = 5
  })
}

resource "aws_sqs_queue" "points_awarded_dlq" {
  name                      = "points-awarded-dlq"
  message_retention_seconds = 1209600
}

resource "aws_sqs_queue" "points_awarded" {
  name                      = "points-awarded"
  message_retention_seconds = 345600
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.points_awarded_dlq.arn
    maxReceiveCount     = 5
  })
}

# A message sitting in a DLQ with nobody watching it is the same as no
# DLQ at all — alarm the instant one shows up rather than relying on
# someone remembering to check.
resource "aws_cloudwatch_metric_alarm" "dlq_not_empty" {
  for_each = {
    session-completed-dlq = aws_sqs_queue.session_completed_dlq.name
    points-awarded-dlq    = aws_sqs_queue.points_awarded_dlq.name
  }

  alarm_name          = "reward-platform-${each.key}-not-empty"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 1
  metric_name         = "ApproximateNumberOfMessagesVisible"
  namespace           = "AWS/SQS"
  period              = 300
  statistic           = "Maximum"
  threshold           = 0
  alarm_actions       = [aws_sns_topic.alarms.arn]

  dimensions = {
    QueueName = each.value
  }
}
