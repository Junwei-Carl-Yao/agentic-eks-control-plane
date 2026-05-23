# Custom HTTPS domain for the EKS-hosted app.
#
# The ALB is reconciled by the AWS Load Balancer Controller from the Ingress
# in deploy/ingress/alb-ingress.yaml, so we deliberately do not declare the
# load balancer in Terraform. We do declare the ACM certificate and the
# Route53 alias that points <subdomain>.<domain_name> at the LBC-managed ALB.
#
# Two-phase apply (only on first setup):
#   1. terraform apply                              # creates cert + DNS validation; ALB does not exist yet.
#   2. make deploy                                  # LBC creates the ALB with the HTTPS listener + cert.
#   3. terraform apply -var='create_dns_alias=true' # discovers the ALB by tag, writes the A-alias.
# Subsequent applies with create_dns_alias=true are idempotent.

locals {
  # Empty domain_name disables every resource in this file — keeps the module
  # backwards-compatible for environments that do not want a custom URL.
  custom_domain_enabled = var.domain_name != ""
  fqdn                  = local.custom_domain_enabled ? "${var.subdomain}.${var.domain_name}" : ""
}

data "aws_route53_zone" "primary" {
  count = local.custom_domain_enabled ? 1 : 0

  zone_id = var.hosted_zone_id
}

resource "aws_acm_certificate" "app" {
  count = local.custom_domain_enabled ? 1 : 0

  domain_name       = local.fqdn
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "cert_validation" {
  for_each = local.custom_domain_enabled ? {
    for option in aws_acm_certificate.app[0].domain_validation_options : option.domain_name => {
      name   = option.resource_record_name
      record = option.resource_record_value
      type   = option.resource_record_type
    }
  } : {}

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = data.aws_route53_zone.primary[0].zone_id
}

resource "aws_acm_certificate_validation" "app" {
  count = local.custom_domain_enabled ? 1 : 0

  certificate_arn         = aws_acm_certificate.app[0].arn
  validation_record_fqdns = [for record in aws_route53_record.cert_validation : record.fqdn]
}

# The LBC tags every ALB it provisions with `ingress.k8s.aws/stack` =
# `<namespace>/<ingress-name>`. We discover the ALB by that tag so the
# Route53 alias survives ALB recreation (the new ALB inherits the tag).
data "aws_lbs" "ingress_alb" {
  count = local.custom_domain_enabled && var.create_dns_alias ? 1 : 0

  tags = {
    "ingress.k8s.aws/stack" = "control-plane/eks-control-plane"
    "elbv2.k8s.aws/cluster" = var.cluster_name
  }
}

data "aws_lb" "ingress_alb" {
  count = local.custom_domain_enabled && var.create_dns_alias ? 1 : 0

  arn = tolist(data.aws_lbs.ingress_alb[0].arns)[0]
}

resource "aws_route53_record" "app_alias" {
  count = local.custom_domain_enabled && var.create_dns_alias ? 1 : 0

  zone_id = data.aws_route53_zone.primary[0].zone_id
  name    = local.fqdn
  type    = "A"

  alias {
    name                   = data.aws_lb.ingress_alb[0].dns_name
    zone_id                = data.aws_lb.ingress_alb[0].zone_id
    evaluate_target_health = true
  }
}
