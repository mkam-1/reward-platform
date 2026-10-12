# Rollback

## A bad deploy (application code)

Every image is tagged with the commit SHA that built it (`:latest` is a
moving pointer, the SHA tag isn't). To roll back a service to the
previous known-good build:

```
aws ecs update-service \
  --cluster reward-platform \
  --service <service> \
  --task-definition reward-platform-<service>:<previous-revision>
```

Each `terraform apply` that changes a task definition creates a new
revision rather than overwriting one — `aws ecs list-task-definitions
--family-prefix reward-platform-<service>` lists them, newest last.

## A bad `terraform apply` (infrastructure)

1. `terraform plan` against the previous commit's `infra/terraform` to
   see exactly what would change back.
2. If the previous state is simple to restore, `terraform apply` from
   that previous commit directly.
3. For a destructive change already applied (a resource recreated with
   data loss, e.g. RDS), restore is no longer a Terraform operation —
   it's an RDS point-in-time-restore or snapshot restore, followed by
   updating Terraform's config to match what actually exists
   (`terraform import` / state surgery) rather than letting the next
   `apply` try to "fix" it by recreating again.

## General principle

CI's `terraform-apply` job requires a manual approval
(`environment: production`) specifically so step 1 above — reviewing
the plan before it lands — has already happened once; a rollback is
the same review, run in reverse.
