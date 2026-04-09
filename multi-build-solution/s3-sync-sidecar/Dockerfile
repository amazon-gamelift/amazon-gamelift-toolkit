FROM public.ecr.aws/amazonlinux/amazonlinux:latest

# Install AWS CLI and basic utilities
RUN yum install -y -q aws-cli shadow-utils findutils && \
    yum clean all

# Copy sync script
COPY sync.sh /sync.sh
RUN chmod +x /sync.sh

# Run as root so we can write to the mounted volume and set ownership
CMD ["/sync.sh"]

