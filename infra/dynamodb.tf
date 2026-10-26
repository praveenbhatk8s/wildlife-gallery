resource "aws_dynamodb_table" "photos" {
  name         = "${var.name}-photos"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "photoId"

  attribute {
    name = "photoId"
    type = "S"
  }
  attribute {
    name = "album"
    type = "S"
  }
  attribute {
    name = "sortKey"
    type = "S"
  }

  # Gallery page (phase 2): list an album newest first.
  global_secondary_index {
    name            = "byAlbum"
    hash_key        = "album"
    range_key       = "sortKey"
    projection_type = "ALL"
  }

  # Needed by the restore-verified-backups project; costs cents at this size.
  point_in_time_recovery {
    enabled = true
  }

  server_side_encryption {
    enabled = true
  }
}
