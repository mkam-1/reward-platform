# One shared ALB, path-routed to all three services — same shape as
# infra/k8s/30-ingress.yaml's Ingress, just AWS's version of the same
# idea: avoid paying for (and managing) three separate load balancers
# when path-based rules on one do the job.
resource "aws_lb" "main" {
  name               = "reward-platform"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = data.aws_subnets.default.ids
}

resource "aws_lb_target_group" "services" {
  for_each = local.services

  name        = "reward-platform-${each.key}"
  port        = each.value.port
  protocol    = "HTTP"
  vpc_id      = data.aws_vpc.default.id
  target_type = "ip" # required for awsvpc-mode Fargate tasks

  health_check {
    path                = "/readyz"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 15
    timeout             = 5
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"

  # Default: anything not matched by a more specific rule below goes to
  # player-service.
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.services["player-service"].arn
  }
}

resource "aws_lb_listener_rule" "games" {
  listener_arn = aws_lb_listener.http.arn
  priority     = 10
  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.services["game-service"].arn
  }
  condition {
    path_pattern {
      values = ["/games", "/games/*"]
    }
  }
}

resource "aws_lb_listener_rule" "rewards" {
  listener_arn = aws_lb_listener.http.arn
  priority     = 20
  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.services["rewards-service"].arn
  }
  condition {
    path_pattern {
      values = ["/rewards", "/rewards/*", "/players/*/redemptions"]
    }
  }
}
