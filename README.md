
# 🛡️ GCP Billing Killswitch

This project provides an automated "circuit breaker" for Google Cloud. When your budget hits 100%, a Cloud Run service automatically unlinks your billing account to prevent further charges.

## 1. Environment Preparation

### Initialize Go

```sh
go mod init gcp-billing-killswitch
go mod tidy
```

### Configure Variables

```sh
# Core Configuration
export GCP_PROJECT_ID="testproj-05sept2022"
export GCP_REGION="us-central1"
export IMG_NAME="gcp-billing-killswitch"
export IMG_VER="v0.1"

# Derived Variables
export GCP_PUBSUB_TOPIC_NAME="billing-alerts"
export GCP_PUBSUB_SUBSCRIPTION_NAME="billing-killswitch-sub"
export GCP_IMAGE="$GCP_REGION-docker.pkg.dev/$GCP_PROJECT_ID/cloud-run-source-deploy/$IMG_NAME:$IMG_VER"
export GCP_CR_SERVICE_NAME="$IMG_NAME"

# Dynamic Resolution
export GCP_PROJECT_NUMBER=$(gcloud projects describe "$GCP_PROJECT_ID" --project "$GCP_PROJECT_ID" --format="get(projectNumber)")
export BILLING_ACCOUNT_ID=$(gcloud billing projects describe "$GCP_PROJECT_ID" --project "$GCP_PROJECT_ID" --format="value(billingAccountName)" | sed 's:.*/::')

printf "Project: %s | Number: %s | Billing ID: %s\n" "$GCP_PROJECT_ID" "$GCP_PROJECT_NUMBER" "$BILLING_ACCOUNT_ID"
```

---

## 2. Build & Deploy

### Container Management

```sh
# Build
podman build -t "$GCP_IMAGE" .

# Authenticate & Push
gcloud auth print-access-token | podman login -u "oauth2accesstoken" --password-stdin "$GCP_REGION-docker.pkg.dev"
podman push "$GCP_IMAGE" --remove-signatures
```

### Cloud Run Deployment

```sh
gcloud run deploy "$GCP_CR_SERVICE_NAME" \
  --project "$GCP_PROJECT_ID" \
  --image "$GCP_IMAGE" \
  --region "$GCP_REGION" \
  --ingress internal \
  --set-env-vars GCP_PROJECT_NUMBER="$GCP_PROJECT_NUMBER" \
  --service-account "$GCP_PROJECT_NUMBER-compute@developer.gserviceaccount.com" \
  --no-allow-unauthenticated
```

---

## 3. Permissions & Wiring

### IAM Permissions

```sh
# Allow Pub/Sub to invoke the service
gcloud run services add-iam-policy-binding "$GCP_CR_SERVICE_NAME" \
  --project "$GCP_PROJECT_ID" \
  --member "serviceAccount:$GCP_PROJECT_NUMBER-compute@developer.gserviceaccount.com" \
  --role "roles/run.invoker" \
  --region "$GCP_REGION"

# Allow Service Account to manage billing at project level
gcloud projects add-iam-policy-binding "$GCP_PROJECT_ID" \
  --project "$GCP_PROJECT_ID" \
  --member "serviceAccount:$GCP_PROJECT_NUMBER-compute@developer.gserviceaccount.com" \
  --role "roles/billing.projectManager"

```

### Pub/Sub Setup

```sh
# Create Topic
gcloud pubsub topics create "$GCP_PUBSUB_TOPIC_NAME" --project "$GCP_PROJECT_ID"

# Allow Google Billing to publish to the topic
gcloud pubsub topics add-iam-policy-binding "$GCP_PUBSUB_TOPIC_NAME" \
  --project "$GCP_PROJECT_ID" \
  --member "serviceAccount:billing-budget-alert@system.gserviceaccount.com" \
  --role "roles/pubsub.publisher"

# Create Push Subscription
SERVICE_URL=$(gcloud run services describe "$GCP_CR_SERVICE_NAME" --project "$GCP_PROJECT_ID" --region "$GCP_REGION" --format="value(status.url)")

gcloud pubsub subscriptions create "$GCP_PUBSUB_SUBSCRIPTION_NAME" \
  --project "$GCP_PROJECT_ID" \
  --topic "$GCP_PUBSUB_TOPIC_NAME" \
  --push-endpoint "$SERVICE_URL" \
  --push-auth-service-account "$GCP_PROJECT_NUMBER-compute@developer.gserviceaccount.com"
```

---

## 4. Link Budget to Killswitch

### Connect Budget to Pub/Sub

```sh
# Get the first budget ID
BUDGET_ID=$(gcloud billing budgets list --billing-account "$BILLING_ACCOUNT_ID" --project "$GCP_PROJECT_ID" --format="value(name)" | head -n 1 | sed 's:.*/::')

# Wire the budget to the topic
gcloud billing budgets update "$BUDGET_ID" \
  --project "$GCP_PROJECT_ID" \
  --billing-account "$BILLING_ACCOUNT_ID" \
  --notifications-rule-pubsub-topic "projects/$GCP_PROJECT_ID/topics/$GCP_PUBSUB_TOPIC_NAME"
```

---

## 5. Testing & Maintenance

### Validation

#### Verify pub/sub notifications rule is present
```sh
gcloud billing budgets describe "$BUDGET_ID" \
--billing-account "$BILLING_ACCOUNT_ID" \
--project "$GCP_PROJECT_ID" \
--format "json" | jq -r ".notificationsRule"
```
```json
{
  "pubsubTopic": "projects/testproj-05sept2022/topics/billing-alerts",
  "schemaVersion": "1.0"
}
```

#### Verify Billing Service Account can publish to Pub/Sub
```sh
# billing-budget-alert@system.gserviceaccount.com needs "roles/pubsub.publisher"
cloud pubsub topics get-iam-policy "$GCP_PUBSUB_TOPIC_NAME" \
--project "$GCP_PROJECT_ID" \
--format "json"
```
```json
{
  "bindings": [
    {
      "members": [
        "serviceAccount:billing-budget-alert@system.gserviceaccount.com"
      ],
      "role": "roles/pubsub.publisher"
    }
  ],
  "etag": "BwZKBeNzwck=",
  "version": 1
}
```

#### Verify Project Number is set inside the running container as an environment varialbe
```sh
gcloud run services describe "$GCP_CR_SERVICE_NAME" \
--project "$GCP_PROJECT_ID" \
--region "$GCP_REGION" \
--format "json" | jq -r ".spec.template.spec.containers[0].env"
```
```json
[
  {
    "name": "GCP_PROJECT_NUMBER",
    "value": "691569880619"
  }
]
```

### End-to-End Simulation Test (Warning!!! This will disabled billing if successful!)

```sh
gcloud pubsub topics publish "$GCP_PUBSUB_TOPIC_NAME" \
  --project "$GCP_PROJECT_ID" \
  --message "$(jq -cn --argjson t 1.0 '{alertThresholdExceeded: $t}')"
```

### Emergency Manual Controls

```sh
# Check Status
gcloud billing projects describe "$GCP_PROJECT_ID" --project "$GCP_PROJECT_ID"

# Manual Unlink
gcloud billing projects unlink "$GCP_PROJECT_ID" --project "$GCP_PROJECT_ID"

# Manual Relink
gcloud billing projects link "$GCP_PROJECT_ID" --project "$GCP_PROJECT_ID" --billing-account "$BILLING_ACCOUNT_ID"

# Read Service Logs
gcloud alpha run services logs read "$GCP_CR_SERVICE_NAME" --project "$GCP_PROJECT_ID" --region "$GCP_REGION"
```

