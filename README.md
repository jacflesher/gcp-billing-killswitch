# gcp-billing-killswitch

### How to set it up

#### 1. Create your script (`script.sh`)

Make sure your script handles the threshold value. Cloud Run Jobs pass information via environment variables or arguments. When triggered by Pub/Sub, the data is usually passed in the request body (for Services) or you can set up a wrapper to pass it as an argument (for Jobs).

```bash
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

```

#### 2. Create the `Containerfile`

You can use a slim image like **Alpine** (which uses `ash`, a POSIX-compliant shell) or a **Google Cloud SDK** image if you want the `gcloud` CLI pre-installed.

```dockerfile
# Use the Google Cloud SDK image to have 'gcloud' ready
FROM google/cloud-sdk:slim

# Copy the script into the container
COPY script.sh /script.sh
RUN chmod +x /script.sh

# Run the script when the job starts
ENTRYPOINT ["/bin/bash", "/script.sh"]

```

#### 3. Build and push the container
```sh
podman build -t us-central1-docker.pkg.dev/testproj-05sept2022/cloud-run-source-deploy/gcp-billing-killswitch:v0.1 .
gcloud auth print-access-token | podman login -u oauth2accesstoken --password-stdin us-central1-docker.pkg.dev
podman push us-central1-docker.pkg.dev/testproj-05sept2022/cloud-run-source-deploy/ytdl:v0.6 --remove-signatures
```