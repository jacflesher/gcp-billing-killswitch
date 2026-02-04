#!/bin/sh
# $1 would be the threshold value passed from the entrypoint
THRESHOLD=$1

echo "Current threshold is: $THRESHOLD"

if [ "$THRESHOLD" = "1.0" ]; then
  echo "100% reached. Running disable-billing command..."
  # You would use gcloud commands here
  gcloud billing projects unlink $GOOGLE_CLOUD_PROJECT
else
  echo "Threshold is below 100%. No action required."
fi