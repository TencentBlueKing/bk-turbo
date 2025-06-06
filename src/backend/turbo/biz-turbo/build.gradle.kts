dependencies {
    api(project(":api-turbo"))
    api(project(":model-turbo"))
    api(project(":common-turbo:common-turbo-client"))
    api(project(":common-turbo:common-turbo-web"))
    api(project(":common-turbo:common-turbo-quartz"))
    api(project(":common-turbo:common-turbo-db"))
    api(project(":common-turbo:common-turbo-util"))
    api("org.springframework.boot:spring-boot-starter-amqp")
    api("com.github.ben-manes.caffeine:caffeine")
    api("com.google.guava:guava")
    api("io.jsonwebtoken:jjwt")
    api("org.jetbrains.kotlinx:kotlinx-coroutines-core")
    api("com.tencent.bk.devops.ci.project:api-project:${Versions.ciVersion}"){
        isTransitive = false
    }
    api("com.tencent.bk.devops.ci.common:common-api:${Versions.ciVersion}"){
        isTransitive = false
    }
    api("com.tencent.bk.devops.ci.auth:api-auth:${Versions.ciVersion}"){
        isTransitive = false
    }
    api("com.tencent.bk.devops.ci.common:common-auth-api:${Versions.ciVersion}"){
        isTransitive = false
    }
    api("com.tencent.bk.devops.ci.metrics:api-metrics") {
        isTransitive = false
    }
}
