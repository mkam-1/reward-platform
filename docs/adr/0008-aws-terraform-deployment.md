# 0008: AWS deployment shape — Fargate, default VPC, one shared ALB

## Context

Phase 11/12 move the platform from local docker-compose/Kubernetes onto
real AWS infrastructure, defined as code (Terraform, wrapped by
Terragrunt for remote state). The account used is a personal AWS free
tier account — cost has to be a first-class constraint on every
resource choice, not an afterthought.

## Decision

- **ECS Fargate**, not EC2 or EKS: no servers to patch or size, and the
  smallest Fargate task (256 CPU / 512 MB) comfortably fits one of
  these services. EKS would teach real Kubernetes-on-AWS, but adds a
  control-plane cost ($0.10/hr, ~$73/month) with no free-tier offset —
  not justified for a learning deployment that already got real k8s
  practice locally in Phase 10.
- **The account's default VPC**, not a dedicated one: Fargate tasks run
  in its public subnets with public IPs, locked down by security
  groups rather than network isolation via private subnets + NAT. A
  NAT gateway bills per-hour regardless of traffic — the one AWS
  primitive that turns "small" into "small but always running," which
  is exactly the free-tier guardrail this project commits to avoiding.
- **One shared Application Load Balancer**, path-routed to all three
  services (`/games*` → game-service, `/rewards*` and
  `/players/*/redemptions` → rewards-service, everything else →
  player-service) — mirrors the Kubernetes Ingress from Phase 10
  exactly, and avoids paying for three ALBs when path rules on one do
  the job.
- **AWS Cloud Map** for service discovery (rewards-service → gRPC →
  player-service), ECS's native equivalent of a Kubernetes Service's
  stable DNS name — the alternative (hardcoding a task IP, or standing
  up an internal NLB) is either fragile or another billed resource.
- **DynamoDB on-demand billing**, not provisioned capacity: no traffic
  forecasting needed, and on-demand is free-tier eligible at this
  scale.
- **RDS `db.t3.micro`, single-AZ, 20GB gp2**: the exact shape of AWS's
  free tier (750 instance-hours/month for 12 months). No Multi-AZ
  standby — the free tier doesn't cover it, and this isn't a production
  SLA commitment.
- **A CloudWatch budget alarm** (80% actual, 100% forecasted) is part
  of the Terraform config itself, not a manual console step taken once
  and forgotten — the guardrail should ship with the infrastructure it
  guards.

## Consequences

- No NAT gateway means tasks need outbound internet access for ECR
  pulls and AWS API calls, which the public-subnet + public-IP +
  security-group design provides — but it also means tasks are
  internet-routable in principle, mitigated only by security groups,
  not network topology. Acceptable for a learning deployment; a real
  production system would push back to private subnets + NAT (or VPC
  endpoints for AWS services specifically, avoiding NAT entirely).
- `desired_count = 1` per service (no HA, no autoscaling on ECS) is a
  deliberate step down from Phase 10's Kubernetes HPA — the learning
  goal there was autoscaling mechanics, not re-proving them a second
  time at real AWS cost.
- `terraform plan`/`apply` were not run against a real AWS account in
  this session (time constraint) — the config is `terraform validate`
  clean and reviewed resource-by-resource, but has not been proven to
  actually provision successfully end-to-end. That remains the
  immediate next step before this is "done," not a formality.

## Alternatives considered

- **EKS**: rejected — see above (control-plane cost, redundant with
  Phase 10's local Kubernetes learning).
- **A dedicated VPC with NAT**: the textbook-correct production
  architecture, rejected here specifically for its always-on cost
  against a free-tier account with no production traffic to justify it.
- **Secrets Manager for all config**: used only for the one real secret
  (the Postgres connection string); everything else is plain
  environment variables in the task definition — Secrets Manager bills
  per secret per month, and most of this config isn't actually
  sensitive.
