export AWS_PROFILE ?= portfolio
FN    := functions/process-photo
TF    := terraform -chdir=infra
ALBUM ?= unsorted

.PHONY: test build init plan apply upload logs photos dlq destroy

test:
	cd $(FN) && go test ./...

# Reproducible build: same source -> same zip hash -> Terraform only redeploys on real changes.
build: test
	mkdir -p dist
	cd $(FN) && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -trimpath -buildvcs=false -ldflags="-s -w" -o ../../dist/bootstrap .
	cd dist && touch -t 202001010000 bootstrap && rm -f process-photo.zip && zip -q -X process-photo.zip bootstrap

init:
	$(TF) init

plan: build
	$(TF) plan -out=tfplan

apply:
	$(TF) apply tfplan

# make upload FILE=~/Pictures/kingfisher.jpg ALBUM=2026-10-thueringer-wald
upload:
	aws s3 cp "$(FILE)" "s3://$$($(TF) output -raw originals_bucket)/$(ALBUM)/"

logs:
	aws logs tail /aws/lambda/wildlife-gallery-process-photo --follow --format short

photos:
	aws dynamodb scan --table-name $$($(TF) output -raw table_name) \
	  --query 'Items[].[album.S,photoId.S,camera.S,lens.S,hadGps.BOOL]' --output table

dlq:
	aws sqs get-queue-attributes --queue-url $$($(TF) output -raw dlq_url) \
	  --attribute-names ApproximateNumberOfMessages

# Two steps: Terraform reads force_destroy from state, so it must be applied before destroy.
destroy:
	$(TF) apply -auto-approve -var allow_destroy_originals=true -target=aws_s3_bucket.originals
	$(TF) destroy
