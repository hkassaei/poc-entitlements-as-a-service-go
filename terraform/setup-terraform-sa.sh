#!/bin/bash
set -e  # Exit immediately if a command exits with a non-zero status

# --- CONFIGURATION ---
# Replace this with your actual Project ID
PROJECT_ID="gcprdpscdpochcppaasdev01-c304"
# Name for the new Service Account
SA_NAME="terraform-runner"
SA_DISPLAY_NAME="Terraform Automation Runner"

# --- 1. CREATE SERVICE ACCOUNT ---
echo "Creating Service Account: $SA_NAME..."

# Check if SA already exists to avoid error
if gcloud iam service-accounts describe "$SA_NAME@$PROJECT_ID.iam.gserviceaccount.com" --project="$PROJECT_ID" > /dev/null 2>&1; then
    echo "Service Account $SA_NAME already exists. Proceeding to role assignment..."
else
    gcloud iam service-accounts create "$SA_NAME" \
        --description="$SA_DISPLAY_NAME" \
        --display-name="$SA_DISPLAY_NAME" \
        --project="$PROJECT_ID"
    echo "Service Account created."
fi

# Define the full Service Account email format
SA_EMAIL="$SA_NAME@$PROJECT_ID.iam.gserviceaccount.com"
IAM_MEMBER="serviceAccount:$SA_EMAIL"

echo "Targeting IAM Member: $IAM_MEMBER"

# --- 2. DEFINE ROLES ---
ROLES=(
  "roles/artifactregistry.admin"
  "roles/cloudkms.admin"
  "roles/vpcaccess.admin"
  "roles/secretmanager.admin"
  "roles/run.admin"
  "roles/iam.serviceAccountAdmin"
  # "roles/resourcemanager.projectIamAdmin"
  "roles/servicenetworking.networksAdmin"
  "roles/redis.admin"
  "roles/cloudsql.admin"
)

# --- 3. ASSIGN ROLES ---
for role in "${ROLES[@]}"; do
  echo "Assigning $role to $SA_NAME..."
  gcloud projects add-iam-policy-binding "$PROJECT_ID" \
    --member="$IAM_MEMBER" \
    --role="$role" \
    --condition=None \
    --quiet > /dev/null # Suppress verbose output for cleaner logs
done

echo "------------------------------------------------"
echo "Success! Service Account '$SA_EMAIL' is ready."
echo "------------------------------------------------"