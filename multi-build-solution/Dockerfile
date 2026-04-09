FROM public.ecr.aws/amazonlinux/amazonlinux:latest

RUN yum install go procps shadow-utils -y -q

# Create shared builds directory mount point (before switching to non-root user)
RUN mkdir -p /shared/builds

# Create and switch to a non root user (Unreal server builds require this)
RUN groupadd --gid 1000 serveruser \
    && useradd --uid 1000 --gid serveruser -m serveruser \
    && usermod -aG wheel serveruser

# Give serveruser access to the shared builds directory
RUN chown -R serveruser:serveruser /shared/builds

USER serveruser

# Copy over the SDK wrapper (no longer copying ServerBuild as it comes from S3)
COPY --chown=serveruser:serveruser SdkGoWrapper/ SdkGoWrapper/

# Copy over the wrapper script and run it as the entry point
COPY --chown=serveruser:serveruser wrapper.sh wrapper.sh

RUN chmod +x ./wrapper.sh

CMD ./wrapper.sh
