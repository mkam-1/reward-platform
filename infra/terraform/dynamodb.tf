# Mirrors the single-table design in
# services/rewards-service/internal/dynamodb/dynamodb.go exactly: reward
# items keyed PK=SK="REWARD#<id>", redemption items keyed
# PK="REWARD#<id>" SK="REDEMPTION#<id>", with GSI1 giving "this player's
# redemption history" an indexed Query instead of a table Scan.
resource "aws_dynamodb_table" "rewards" {
  name         = "reward_platform"
  billing_mode = "PAY_PER_REQUEST" # no capacity planning, and free-tier eligible at this traffic level

  hash_key  = "PK"
  range_key = "SK"

  attribute {
    name = "PK"
    type = "S"
  }
  attribute {
    name = "SK"
    type = "S"
  }
  attribute {
    name = "GSI1PK"
    type = "S"
  }
  attribute {
    name = "GSI1SK"
    type = "S"
  }

  global_secondary_index {
    name            = "GSI1"
    hash_key        = "GSI1PK"
    range_key       = "GSI1SK"
    projection_type = "ALL"
  }
}
