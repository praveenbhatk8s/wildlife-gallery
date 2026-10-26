resource "aws_cloudfront_origin_access_control" "derived" {
  name                              = "${var.name}-derived"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

data "aws_cloudfront_cache_policy" "optimized" {
  name = "Managed-CachingOptimized"
}

resource "aws_cloudfront_distribution" "gallery" {
  enabled         = true
  comment         = "Wildlife gallery - derived images"
  price_class     = "PriceClass_100" # Europe + North America edges only, cheapest
  is_ipv6_enabled = true
  http_version    = "http2and3"

  origin {
    origin_id                = "derived"
    domain_name              = aws_s3_bucket.derived.bucket_regional_domain_name
    origin_access_control_id = aws_cloudfront_origin_access_control.derived.id
  }

  default_cache_behavior {
    target_origin_id       = "derived"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
    cache_policy_id        = data.aws_cloudfront_cache_policy.optimized.id
    compress               = true
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
  }
}
