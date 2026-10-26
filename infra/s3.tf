# ---------- Originals: private, versioned, cheap storage after 30 days ----------
resource "aws_s3_bucket" "originals" {
  bucket        = local.originals_name
  force_destroy = var.allow_destroy_originals
}

resource "aws_s3_bucket_public_access_block" "originals" {
  bucket                  = aws_s3_bucket.originals.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "originals" {
  bucket = aws_s3_bucket.originals.id
  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_versioning" "originals" {
  bucket = aws_s3_bucket.originals.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "originals" {
  bucket = aws_s3_bucket.originals.id

  rule {
    id     = "originals-to-glacier-ir"
    status = "Enabled"
    filter {}

    # Glacier Instant Retrieval: much cheaper than Standard, still millisecond reads
    # if a photo ever needs reprocessing. Minimum billable duration is 90 days.
    transition {
      days          = 30
      storage_class = "GLACIER_IR"
    }

    noncurrent_version_expiration {
      noncurrent_days = 30
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

# ---------- Derived: web sizes only, no metadata, served via CloudFront ----------
resource "aws_s3_bucket" "derived" {
  bucket        = local.derived_name
  force_destroy = true # everything here can be regenerated from the originals
}

resource "aws_s3_bucket_public_access_block" "derived" {
  bucket                  = aws_s3_bucket.derived.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "derived" {
  bucket = aws_s3_bucket.derived.id
  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

# ---------- Bucket policies ----------
data "aws_iam_policy_document" "originals" {
  statement {
    sid       = "DenyInsecureTransport"
    effect    = "Deny"
    actions   = ["s3:*"]
    resources = [aws_s3_bucket.originals.arn, "${aws_s3_bucket.originals.arn}/*"]
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3_bucket_policy" "originals" {
  bucket     = aws_s3_bucket.originals.id
  policy     = data.aws_iam_policy_document.originals.json
  depends_on = [aws_s3_bucket_public_access_block.originals]
}

data "aws_iam_policy_document" "derived" {
  statement {
    sid       = "CloudFrontReadOnly"
    actions   = ["s3:GetObject"]
    resources = ["${aws_s3_bucket.derived.arn}/*"]
    principals {
      type        = "Service"
      identifiers = ["cloudfront.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "AWS:SourceArn"
      values   = [aws_cloudfront_distribution.gallery.arn]
    }
  }

  statement {
    sid       = "DenyInsecureTransport"
    effect    = "Deny"
    actions   = ["s3:*"]
    resources = [aws_s3_bucket.derived.arn, "${aws_s3_bucket.derived.arn}/*"]
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3_bucket_policy" "derived" {
  bucket     = aws_s3_bucket.derived.id
  policy     = data.aws_iam_policy_document.derived.json
  depends_on = [aws_s3_bucket_public_access_block.derived]
}
