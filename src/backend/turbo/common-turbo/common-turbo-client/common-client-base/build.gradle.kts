dependencies {
    api(project(":common-turbo:common-turbo-service"))
    api(project(":common-turbo:common-turbo-api"))
    api(project(":common-turbo:common-turbo-util"))
    api(project(":common-turbo:common-turbo-security"))
    api("io.github.openfeign:feign-jaxrs")
    api("io.github.openfeign:feign-okhttp")
    api("io.github.openfeign:feign-jackson")
}
