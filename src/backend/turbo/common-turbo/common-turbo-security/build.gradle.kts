dependencies {
    api(project(":common-turbo:common-turbo-util"))
    api("org.apache.commons:commons-lang3")
    api("org.springframework.boot:spring-boot-autoconfigure")
    api("io.jsonwebtoken:jjwt-api")
    api("com.google.guava:guava")
    runtimeOnly("io.jsonwebtoken:jjwt-impl")
    runtimeOnly("io.jsonwebtoken:jjwt-jackson")
}
