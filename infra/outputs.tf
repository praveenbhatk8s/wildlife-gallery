output "originals_bucket" {
  value = aws_s3_bucket.originals.bucket
}

output "derived_bucket" {
  value = aws_s3_bucket.derived.bucket
}

output "cdn_domain" {
  description = "Photos are served at https://<cdn_domain>/photos/<photoId>/{thumb,web}.jpg"
  value       = aws_cloudfront_distribution.gallery.domain_name
}

output "table_name" {
  value = aws_dynamodb_table.photos.name
}

output "function_name" {
  value = aws_lambda_function.process.function_name
}

output "dlq_url" {
  value = aws_sqs_queue.dlq.url
}
