output "region" {
  description = "AWS region the cluster lives in."
  value       = var.region
}

output "cluster_name" {
  description = "EKS cluster name."
  value       = module.eks.cluster_name
}

output "cluster_endpoint" {
  description = "EKS cluster API server endpoint."
  value       = module.eks.cluster_endpoint
}

output "cluster_certificate_authority_data" {
  description = "Base64-encoded cluster CA (for kubeconfig)."
  value       = module.eks.cluster_certificate_authority_data
}

output "oidc_issuer" {
  description = "OIDC issuer URL for IRSA."
  value       = module.eks.oidc_provider_url
}

output "oidc_provider_arn" {
  description = "ARN of the IAM OIDC provider registered for the cluster."
  value       = module.eks.oidc_provider_arn
}

output "node_group_arn" {
  description = "ARN of the managed node group."
  value       = module.eks.node_group_arn
}

output "vpc_id" {
  description = "ID of the VPC hosting the cluster."
  value       = module.vpc.vpc_id
}

output "private_subnet_ids" {
  description = "Private subnet IDs where node groups run."
  value       = module.vpc.private_subnet_ids
}

output "public_subnet_ids" {
  description = "Public subnet IDs used for internet-facing load balancers."
  value       = module.vpc.public_subnet_ids
}

output "state_bucket_name" {
  description = "S3 bucket backing Terraform remote state."
  value       = var.state_bucket_name
}

output "cluster_role_name" {
  description = "Name of the IAM role the EKS control plane assumes."
  value       = module.iam.cluster_role_name
}

output "node_role_name" {
  description = "Name of the IAM role attached to managed node group instances."
  value       = module.iam.node_role_name
}

output "irsa_backend_role_name" {
  description = "Name of the IRSA role assumable by the backend ServiceAccount."
  value       = module.iam.irsa_backend_role_name
}

output "irsa_backend_role_arn" {
  description = "ARN of the IRSA role assumable by the backend ServiceAccount."
  value       = module.iam.irsa_backend_role_arn
}

output "irsa_lbc_role_arn" {
  description = "ARN of the IRSA role assumable by the AWS Load Balancer Controller ServiceAccount."
  value       = module.iam.irsa_lbc_role_arn
}

output "backend_service_account" {
  description = "Fully-qualified ServiceAccount (namespace:name) pinned in the IRSA trust policy."
  value       = "${var.backend_namespace}:${var.backend_service_account}"
}

output "kubeconfig_command" {
  description = "Shell command to update the local kubeconfig for this cluster."
  value       = "aws eks update-kubeconfig --name ${module.eks.cluster_name} --region ${var.region}"
}
