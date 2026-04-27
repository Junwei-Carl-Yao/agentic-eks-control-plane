locals {
  # Strip the scheme from the OIDC issuer URL so it can be used in the
  # StringEquals sub condition (EKS emits "https://oidc.eks..../id/XXXX").
  oidc_host = replace(var.oidc_provider_url, "https://", "")
}

# ---------------------------------------------------------------------------
# Cluster service role
# ---------------------------------------------------------------------------
data "aws_iam_policy_document" "cluster_assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["eks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "cluster" {
  name               = "${var.cluster_name}-cluster-role"
  assume_role_policy = data.aws_iam_policy_document.cluster_assume.json
}

resource "aws_iam_role_policy_attachment" "cluster_policy" {
  role       = aws_iam_role.cluster.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}

# ---------------------------------------------------------------------------
# Node group role
# ---------------------------------------------------------------------------
data "aws_iam_policy_document" "node_assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "node" {
  name               = "${var.cluster_name}-node-role"
  assume_role_policy = data.aws_iam_policy_document.node_assume.json
}

resource "aws_iam_role_policy_attachment" "node_worker" {
  role       = aws_iam_role.node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
}

resource "aws_iam_role_policy_attachment" "node_ecr" {
  role       = aws_iam_role.node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

resource "aws_iam_role_policy_attachment" "node_cni" {
  role       = aws_iam_role.node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
}

# ---------------------------------------------------------------------------
# IRSA role for the backend pod
#
# Trust: federated on the cluster's OIDC provider, pinned to a single
# namespace:serviceaccount via StringEquals on sub.
# Permissions: read-only EKS describes + read-only access to the Terraform
# state bucket. No ec2:*, no iam:*, no wildcards.
# ---------------------------------------------------------------------------
data "aws_iam_policy_document" "irsa_backend_assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [var.oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_host}:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_host}:sub"
      values   = ["system:serviceaccount:${var.backend_namespace}:${var.backend_service_account}"]
    }
  }
}

resource "aws_iam_role" "irsa_backend" {
  name               = "${var.cluster_name}-irsa-backend"
  assume_role_policy = data.aws_iam_policy_document.irsa_backend_assume.json
}

data "aws_iam_policy_document" "irsa_backend_policy" {
  # Read-only EKS describes scoped to this cluster.
  statement {
    sid    = "EksReadOnlyThisCluster"
    effect = "Allow"
    actions = [
      "eks:DescribeCluster",
      "eks:DescribeNodegroup",
      "eks:DescribeUpdate",
      "eks:ListNodegroups",
      "eks:ListUpdates",
    ]
    resources = [
      "arn:aws:eks:*:*:cluster/${var.cluster_name}",
      "arn:aws:eks:*:*:nodegroup/${var.cluster_name}/*/*",
    ]
  }

  # Listing clusters has no resource-level scoping in IAM; this is the one
  # place we accept Resource:"*" — with no wildcard in Action.
  statement {
    sid       = "EksListClusters"
    effect    = "Allow"
    actions   = ["eks:ListClusters"]
    resources = ["*"]
  }

  # Read-only access to the Terraform state bucket so the backend can show
  # drift/state without being able to write.
  statement {
    sid       = "TfStateBucketList"
    effect    = "Allow"
    actions   = ["s3:ListBucket", "s3:GetBucketLocation"]
    resources = ["arn:aws:s3:::${var.state_bucket_name}"]
  }

  statement {
    sid       = "TfStateObjectsRead"
    effect    = "Allow"
    actions   = ["s3:GetObject", "s3:GetObjectVersion"]
    resources = ["arn:aws:s3:::${var.state_bucket_name}/*"]
  }
}

resource "aws_iam_role_policy" "irsa_backend" {
  name   = "${var.cluster_name}-irsa-backend-policy"
  role   = aws_iam_role.irsa_backend.id
  policy = data.aws_iam_policy_document.irsa_backend_policy.json
}
