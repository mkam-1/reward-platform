resource "aws_ecr_repository" "services" {
  for_each = toset(["player-service", "game-service", "rewards-service"])

  name                 = "reward-platform/${each.key}"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
}

# Keep only the last 10 images per repo — ECR storage is cheap but not
# free, and a CI pipeline pushing on every merge adds up fast with no
# lifecycle policy at all.
resource "aws_ecr_lifecycle_policy" "services" {
  for_each   = aws_ecr_repository.services
  repository = each.value.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "keep last 10 images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 10
      }
      action = { type = "expire" }
    }]
  })
}
