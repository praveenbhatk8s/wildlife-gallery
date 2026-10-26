resource "aws_cloudwatch_log_group" "process" {
  name              = "/aws/lambda/${local.function_name}"
  retention_in_days = 14
}

data "aws_iam_policy_document" "lambda_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "process" {
  name               = "${local.function_name}-role"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

# Least privilege: read originals, write derived, one table, its own logs, its own DLQ.
data "aws_iam_policy_document" "process" {
  statement {
    sid       = "ReadOriginals"
    actions   = ["s3:GetObject"]
    resources = ["${aws_s3_bucket.originals.arn}/*"]
  }
  statement {
    sid       = "WriteDerived"
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.derived.arn}/photos/*"]
  }
  statement {
    sid       = "WritePhotoItems"
    actions   = ["dynamodb:PutItem"]
    resources = [aws_dynamodb_table.photos.arn]
  }
  statement {
    sid       = "OwnLogs"
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.process.arn}:*"]
  }
  statement {
    sid       = "FailureDestination"
    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.dlq.arn]
  }
}

resource "aws_iam_role_policy" "process" {
  name   = "process-photo"
  role   = aws_iam_role.process.id
  policy = data.aws_iam_policy_document.process.json
}

resource "aws_lambda_function" "process" {
  function_name    = local.function_name
  role             = aws_iam_role.process.arn
  filename         = var.lambda_zip
  source_code_hash = filebase64sha256(var.lambda_zip)
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = ["arm64"]
  memory_size      = 1024 # a 24 MP JPEG decodes to ~100 MB of pixels; more memory also means more CPU
  timeout          = 60

  environment {
    variables = {
      DERIVED_BUCKET = aws_s3_bucket.derived.bucket
      TABLE_NAME     = aws_dynamodb_table.photos.name
    }
  }

  logging_config {
    log_format = "JSON"
    log_group  = aws_cloudwatch_log_group.process.name
  }

  depends_on = [aws_iam_role_policy.process]
}

# S3 invokes Lambda asynchronously: 2 retries, then the event goes to the DLQ.
resource "aws_lambda_function_event_invoke_config" "process" {
  function_name                = aws_lambda_function.process.function_name
  maximum_retry_attempts       = 2
  maximum_event_age_in_seconds = 3600

  destination_config {
    on_failure {
      destination = aws_sqs_queue.dlq.arn
    }
  }
}

resource "aws_lambda_permission" "s3_invoke" {
  statement_id   = "AllowOriginalsBucketInvoke"
  action         = "lambda:InvokeFunction"
  function_name  = aws_lambda_function.process.function_name
  principal      = "s3.amazonaws.com"
  source_arn     = aws_s3_bucket.originals.arn
  source_account = local.account_id
}

# Trigger only on the originals bucket. The function writes to a different bucket,
# so it can never re-trigger itself.
resource "aws_s3_bucket_notification" "originals" {
  bucket = aws_s3_bucket.originals.id

  dynamic "lambda_function" {
    for_each = local.photo_suffixes
    content {
      lambda_function_arn = aws_lambda_function.process.arn
      events              = ["s3:ObjectCreated:*"]
      filter_suffix       = lambda_function.value
    }
  }

  depends_on = [aws_lambda_permission.s3_invoke]
}
