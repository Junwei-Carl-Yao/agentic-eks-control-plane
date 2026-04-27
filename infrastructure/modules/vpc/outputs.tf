output "vpc_id" {
  description = "ID of the VPC."
  value       = aws_vpc.this.id
}

output "public_subnet_ids" {
  description = "IDs of the public subnets (tagged for internet-facing ELBs)."
  value       = aws_subnet.public[*].id
}

output "private_subnet_ids" {
  description = "IDs of the private subnets (tagged for internal ELBs; used by node groups)."
  value       = aws_subnet.private[*].id
}

output "nat_gateway_id" {
  description = "ID of the NAT gateway used by private subnets."
  value       = aws_nat_gateway.this.id
}

output "internet_gateway_id" {
  description = "ID of the internet gateway used by public subnets."
  value       = aws_internet_gateway.this.id
}
