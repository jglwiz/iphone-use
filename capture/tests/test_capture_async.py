import asyncio
import json
import unittest
from iphone_use_capture.capture_worker import handle
from iphone_use_capture.frame_snapshot import FrameStore

class CaptureHTTP(unittest.IsolatedAsyncioTestCase):
    async def test_static_frame_disconnect_and_recovery(self):
        # An old snapshot remains observable, without its clock/ID being refreshed.
        import time
        screen = FrameStore()
        old = screen.publish(b'\xff\xd8old\xff\xd9', time.time() - 120)
        server = await asyncio.start_server(lambda r,w:handle(screen,r,w),'127.0.0.1',0)
        port = server.sockets[0].getsockname()[1]
        async def get(path):
            r,w = await asyncio.open_connection('127.0.0.1',port)
            w.write(f'GET {path} HTTP/1.1\r\nHost: localhost\r\n\r\n'.encode())
            await w.drain()
            response = await r.read()
            w.close()
            await w.wait_closed()
            return response
        try:
            first = await get('/frame')
            self.assertEqual(first, await get('/frame'))
            self.assertIn(str(old.timestamp).encode(), first)
            self.assertIn(b'200 OK', first)
            health = json.loads((await get('/health')).split(b'\r\n\r\n',1)[1])
            self.assertTrue(health['has_frame'])
            self.assertEqual(health['mirroring_state'], 'unknown')
            # Pipe EOF explicitly clears the snapshot; no age-based disconnect inference.
            screen.clear()
            self.assertIn(b'204 No Content', await get('/frame'))
            new = screen.publish(b'\xff\xd8new\xff\xd9', time.time())
            self.assertNotEqual(new.frame_id, old.frame_id)
            self.assertTrue((await get('/frame')).endswith(new.jpeg))
            self.assertIn(b'404 Not Found', await get('/action'))
        finally:
            server.close()
            await server.wait_closed()

    async def test_atomic_snapshot_and_no_frame(self):
        screen=FrameStore()
        server=await asyncio.start_server(lambda r,w:handle(screen,r,w),'127.0.0.1',0)
        port=server.sockets[0].getsockname()[1]
        async def request():
            r,w=await asyncio.open_connection('127.0.0.1',port)
            w.write(b'GET /frame HTTP/1.1\r\nHost: localhost\r\n\r\n')
            await w.drain()
            result=await r.read()
            w.close()
            await w.wait_closed()
            return result
        try:
            self.assertIn(b'204 No Content',await request())
            frame=screen.publish(b'\xff\xd8\xff\xd9',123.0)
            responses=await asyncio.gather(*(request() for _ in range(20)))
            for result in responses:
                self.assertIn(frame.frame_id.encode(),result)
                self.assertIn(b'X-Frame-Time: 123.0',result)
                self.assertTrue(result.endswith(frame.jpeg))
        finally:
            server.close()
            await server.wait_closed()
