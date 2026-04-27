output "cluster_name" {
  description = "Name of the EKS cluster."
  value       = aws_eks_cluster.this.name
}

output "cluster_endpoint" {
  description = "Kubernetes API server endpoint."
  value       = aws_eks_cluster.this.endpoint
}

output "cluster_certificate_authority_data" {
  description = "Base64-encoded cluster CA certificate."
  value       = aws_eks_cluster.this.certificate_authority[0].data
}

output "cluster_security_group_id" {
  description = "EKS-managed cluster security group."
  value       = aws_eks_cluster.this.vpc_config[0].cluster_security_group_id
}

output "oidc_provider_arn" {
  description = "ARN of the IAM OIDC identity provider registered for the cluster."
  value       = aws_iam_openid_connect_provider.oidc.arn
}

output "oidc_provider_url" {
  description = "OIDC issuer URL as emitted by EKS, including the https:// prefix."
  value       = aws_eks_cluster.this.identity[0].oidc[0].issuer
}

output "node_group_arn" {
  description = "ARN of the default managed node group."
  value       = aws_eks_node_group.default.arn
}

output "node_group_name" {
  description = "Name of the default managed node group."
  value       = aws_eks_node_group.default.node_group_name
}
