output "cluster_role_arn" {
  description = "ARN of the cluster service role."
  value       = aws_iam_role.cluster.arn
}

output "cluster_role_name" {
  description = "Name of the cluster service role."
  value       = aws_iam_role.cluster.name
}

output "node_role_arn" {
  description = "ARN of the managed node group IAM role."
  value       = aws_iam_role.node.arn
}

output "node_role_name" {
  description = "Name of the managed node group IAM role."
  value       = aws_iam_role.node.name
}

output "irsa_backend_role_arn" {
  description = "ARN of the backend IRSA role."
  value       = aws_iam_role.irsa_backend.arn
}

output "irsa_backend_role_name" {
  description = "Name of the backend IRSA role."
  value       = aws_iam_role.irsa_backend.name
}

output "irsa_lbc_role_arn" {
  description = "ARN of the IRSA role assumable by the AWS Load Balancer Controller ServiceAccount."
  value       = aws_iam_role.lbc.arn
}

output "irsa_lbc_role_name" {
  description = "Name of the IRSA role assumable by the AWS Load Balancer Controller ServiceAccount."
  value       = aws_iam_role.lbc.name
}
