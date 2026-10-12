resource "aws_sqs_queue" "session_completed" {
  name                      = "session-completed"
  message_retention_seconds = 345600 # 4 days
}

resource "aws_sqs_queue" "points_awarded" {
  name                      = "points-awarded"
  message_retention_seconds = 345600
}
