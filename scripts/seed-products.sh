#!/bin/bash
# Seed product master data into Pub/Sub for the demo.
# Run from your Mac before starting the chaos demo.
# Usage: bash scripts/seed-products.sh

PROJECT="retailedge-proxy"
TOPIC="mdm-product-changes"

echo "Seeding product data to Pub/Sub..."

publish() {
  gcloud pubsub topics publish ${TOPIC} \
    --project=${PROJECT} \
    --message="$1"
  echo "  published: $1" | head -c 60
  echo "..."
}

publish '{"product_id":"P001","name":"Organic Milk 2L","price":89.00,"category":"dairy","in_stock":true,"version":1,"event_id":"seed-001","event_type":"PRODUCT_CREATED"}'
publish '{"product_id":"P002","name":"Whole Wheat Bread","price":45.00,"category":"bakery","in_stock":true,"version":1,"event_id":"seed-002","event_type":"PRODUCT_CREATED"}'
publish '{"product_id":"P003","name":"Orange Juice 1L","price":65.00,"category":"beverages","in_stock":true,"version":1,"event_id":"seed-003","event_type":"PRODUCT_CREATED"}'
publish '{"product_id":"P004","name":"Greek Yogurt 500g","price":120.00,"category":"dairy","in_stock":true,"version":1,"event_id":"seed-004","event_type":"PRODUCT_CREATED"}'
publish '{"product_id":"P005","name":"Brown Rice 1kg","price":55.00,"category":"grains","in_stock":true,"version":1,"event_id":"seed-005","event_type":"PRODUCT_CREATED"}'

echo ""
echo "Done. Events Service will sync these to the Near Cache within seconds."