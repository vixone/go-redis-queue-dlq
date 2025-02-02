<?php

use Illuminate\Support\Facades\Http;
use WebSocket\Client;

class GoQueueService
{
    protected $client;

    protected $goQueueUrl;

    public function __construct()
    {
        $this->client = new Client('ws://go-queue-server:8080'); // WebSocket URL of the Go Queue server
        $this->goQueueUrl = env('GO_QUEUE_URL', 'http://localhost:8080/publish');
    }

    public function listenToQueue()
    {
        while (true) {
            $message = $this->client->receive();  // Receive messages from the queue

            if ($message) {
                $this->handleMessage($message);
            }
        }
    }

    public function publishToQueue($event, $data)
    {
        $response = Http::post($this->goQueueUrl, [
            'event' => $event,
            'data' => json_encode($data),
        ]);

        return $response->successful();
    }

    protected function handleMessage($message)
    {
        // Here, you can process the message, log it, or trigger actions in Laravel
        // You could dispatch a Laravel event or queue job to handle the message
        logger('Received message from Go Queue: '.$message);
    }
}
