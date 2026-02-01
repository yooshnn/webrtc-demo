import asyncio
import random
import logging
import grpc
import media_pb2
import media_pb2_grpc

logging.basicConfig(
    level=logging.INFO, 
    format='%(asctime)s [%(levelname)s] %(message)s'
)

class AIProcessorService(media_pb2_grpc.AIProcessorServicer):
    async def ProcessStream(self, request_iterator, context):
        peer = context.peer()
        logging.info(f"New connection from {peer}")
        
        try:
            async for packet in request_iterator:
                # Simulate AI processing with variable delay
                delay = 0.01 if packet.type == 0 else random.uniform(0.05, 0.15)
                await asyncio.sleep(delay)
                
                # Echo packet back (preserve ID for header tracking)
                yield media_pb2.MediaPacket(
                    id=packet.id,
                    type=packet.type,
                    payload=packet.payload
                )
                
        except Exception as e:
            logging.error(f"Stream error: {e}")
        finally:
            logging.info(f"Connection closed: {peer}")

async def serve():
    server = grpc.aio.server()
    media_pb2_grpc.add_AIProcessorServicer_to_server(AIProcessorService(), server)
    
    listen_addr = '[::]:50051'
    server.add_insecure_port(listen_addr)
    logging.info(f"AI Processor Server started on {listen_addr}")
    
    await server.start()
    await server.wait_for_termination()

if __name__ == '__main__':
    asyncio.run(serve())