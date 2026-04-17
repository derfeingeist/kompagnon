# Getting Started

### Reference Documentation
For further reference, please consider the following sections:

* [Official Gradle documentation](https://docs.gradle.org)
* [Spring Boot Gradle Plugin Reference Guide](https://docs.spring.io/spring-boot/4.0.5/gradle-plugin)
* [Create an OCI image](https://docs.spring.io/spring-boot/4.0.5/gradle-plugin/packaging-oci-image.html)
* [Spring Boot Testcontainers support](https://docs.spring.io/spring-boot/4.0.5/reference/testing/testcontainers.html#testing.testcontainers)
* [Testcontainers Grafana Module Reference Guide](https://java.testcontainers.org/modules/grafana/)
* [Testcontainers Ollama Module Reference Guide](https://java.testcontainers.org/modules/testcontainers/)
* [Docker Compose Support](https://docs.spring.io/spring-boot/4.0.5/reference/features/dev-services.html#features.dev-services.docker-compose)
* [Spring Boot DevTools](https://docs.spring.io/spring-boot/4.0.5/reference/using/devtools.html)
* [SpringDoc OpenAPI](https://springdoc.org/)
* [Spring Security](https://docs.spring.io/spring-boot/4.0.5/reference/web/spring-security.html)
* [Spring Data JPA](https://docs.spring.io/spring-boot/4.0.5/reference/data/sql.html#data.sql.jpa-and-spring-data)
* [Flyway Migration](https://docs.spring.io/spring-boot/4.0.5/how-to/data-initialization.html#howto.data-initialization.migration-tool.flyway)
* [Validation](https://docs.spring.io/spring-boot/4.0.5/reference/io/validation.html)
* [Spring Boot Actuator](https://docs.spring.io/spring-boot/4.0.5/reference/actuator/index.html)
* [Prometheus](https://docs.spring.io/spring-boot/4.0.5/reference/actuator/metrics.html#actuator.metrics.export.prometheus)
* [OpenTelemetry](https://docs.spring.io/spring-boot/4.0.5/reference/actuator/observability.html#actuator.observability.opentelemetry)
* [Testcontainers](https://java.testcontainers.org/)
* [Ollama](https://docs.spring.io/spring-ai/reference/api/chat/ollama-chat.html)

### Guides
The following guides illustrate how to use some features concretely:

* [SpringDoc OpenAPI](https://github.com/springdoc/springdoc-openapi-demos/)
* [Securing a Web Application](https://spring.io/guides/gs/securing-web/)
* [Spring Boot and OAuth2](https://spring.io/guides/tutorials/spring-boot-oauth2/)
* [Authenticating a User with LDAP](https://spring.io/guides/gs/authenticating-ldap/)
* [Accessing Data with JPA](https://spring.io/guides/gs/accessing-data-jpa/)
* [Validation](https://spring.io/guides/gs/validating-form-input/)
* [Building a RESTful Web Service with Spring Boot Actuator](https://spring.io/guides/gs/actuator-service/)

### Additional Links
These additional references should also help you:

* [Gradle Build Scans – insights for your project's build](https://scans.gradle.com#gradle)

### Docker Compose support
This project contains a Docker Compose file named `compose.yaml`.
In this file, the following services have been defined:

* grafana-lgtm: [`grafana/otel-lgtm:latest`](https://hub.docker.com/r/grafana/otel-lgtm)
* ollama: [`ollama/ollama:latest`](https://hub.docker.com/r/ollama/ollama)

Please review the tags of the used images and set them to the same as you're running in production.

### Testcontainers support

This project uses [Testcontainers at development time](https://docs.spring.io/spring-boot/4.0.5/reference/features/dev-services.html#features.dev-services.testcontainers).

Testcontainers has been configured to use the following Docker images:

* [`grafana/otel-lgtm:latest`](https://hub.docker.com/r/grafana/otel-lgtm)
* [`ollama/ollama:latest`](https://hub.docker.com/r/ollama/ollama)

Please review the tags of the used images and set them to the same as you're running in production.

