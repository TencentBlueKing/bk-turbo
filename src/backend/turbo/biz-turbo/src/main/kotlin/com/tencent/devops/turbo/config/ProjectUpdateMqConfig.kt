package com.tencent.devops.turbo.config

import com.tencent.devops.common.util.constants.EXCHANGE_PROJECT_ENABLE_FANOUT
import com.tencent.devops.common.util.constants.QUEUE_PROJECT_STATUS_UPDATE
import com.tencent.devops.common.web.mq.CORE_CONNECTION_FACTORY_NAME
import com.tencent.devops.turbo.component.ProjectStatusUpdateConsumer
import org.springframework.amqp.core.Binding
import org.springframework.amqp.core.BindingBuilder
import org.springframework.amqp.core.FanoutExchange
import org.springframework.amqp.core.Queue
import org.springframework.amqp.rabbit.connection.ConnectionFactory
import org.springframework.amqp.rabbit.listener.SimpleMessageListenerContainer
import org.springframework.amqp.rabbit.listener.adapter.MessageListenerAdapter
import org.springframework.amqp.support.converter.Jackson2JsonMessageConverter
import org.springframework.beans.factory.annotation.Qualifier
import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration


@Configuration
class ProjectUpdateMqConfig {

    @Bean
    fun projectStatusUpdateExchange(): FanoutExchange {
        val fanoutExchange = FanoutExchange(EXCHANGE_PROJECT_ENABLE_FANOUT, true, false)
        fanoutExchange.isDelayed = true
        return fanoutExchange
    }

    @Bean
    fun projectStatusUpdateQueue(): Queue {
        return Queue(QUEUE_PROJECT_STATUS_UPDATE, true)
    }

    @Bean
    fun projectStatusUpdateBinding(
        projectStatusUpdateQueue: Queue,
        projectStatusUpdateExchange: FanoutExchange
    ): Binding {
        return BindingBuilder.bind(projectStatusUpdateQueue).to(projectStatusUpdateExchange)
    }

    @Bean
    fun messageListenerContainer(
        @Qualifier(CORE_CONNECTION_FACTORY_NAME)
        connectionFactory: ConnectionFactory,
        projectStatusUpdateQueue: Queue,
        projectStatusUpdateConsumer: ProjectStatusUpdateConsumer,
        messageConverter: Jackson2JsonMessageConverter
    ): SimpleMessageListenerContainer {
        val container = SimpleMessageListenerContainer(connectionFactory)
        container.setPrefetchCount(1)
        container.setConcurrentConsumers(1)
        container.setMaxConcurrentConsumers(1)
        container.setQueueNames(projectStatusUpdateQueue.name)
        val adapter = MessageListenerAdapter(projectStatusUpdateConsumer, projectStatusUpdateConsumer::consumer.name)
        adapter.setMessageConverter(messageConverter)
        container.setMessageListener(adapter)
        return container
    }

}
