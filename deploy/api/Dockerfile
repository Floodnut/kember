FROM gcr.io/distroless/java17-debian12:nonroot

COPY --chown=nonroot:nonroot kember-api_deploy.jar /app/kember-api.jar

ENTRYPOINT ["java", "-cp", "/app/kember-api.jar", "dev.kember.api.MainKt"]
