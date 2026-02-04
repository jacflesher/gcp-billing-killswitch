# Use the Google Cloud SDK image to have 'gcloud' ready
FROM google/cloud-sdk:slim

# Copy the script into the container
COPY script.sh /script.sh
RUN chmod +x /script.sh

# Run the script when the job starts
ENTRYPOINT ["/bin/bash", "/script.sh"]