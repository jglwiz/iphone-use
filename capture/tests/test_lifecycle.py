import asyncio
import os
import tempfile
import unittest
from unittest.mock import patch

from iphone_use_capture.capture_worker import serve_async


class Startup(unittest.IsolatedAsyncioTestCase):
    async def test_occupied_http_does_not_touch_capture(self):
        listener = await asyncio.start_server(lambda r,w:w.close(), '127.0.0.1', 0)
        class Screen:
            def start(self):
                raise AssertionError('must bind before touching pipe')
        try:
            with self.assertRaises(OSError):
                await serve_async(Screen(), listener.sockets[0].getsockname()[1])
        finally:
            listener.close()
            await listener.wait_closed()


@unittest.skipUnless(os.name == 'nt', 'Windows receiver lifecycle')
class Receiver(unittest.TestCase):
    def test_attach_never_writes_config_or_starts_receiver(self):
        os.environ.setdefault('UXPLAY_DIR', tempfile.gettempdir())
        from iphone_use_capture.screen_pipe import Screen
        with patch('iphone_use_capture.screen_pipe._pids_of', return_value=set()), \
             patch.object(Screen, '_write_config') as config, \
             patch('iphone_use_capture.screen_pipe.subprocess.Popen') as spawn:
            Screen(start_receiver=False)._ensure_receiver()
            config.assert_not_called()
            spawn.assert_not_called()

    def test_reuses_receiver_without_changing_session(self):
        os.environ.setdefault('UXPLAY_DIR', tempfile.gettempdir())
        from iphone_use_capture.screen_pipe import Screen
        with patch('iphone_use_capture.screen_pipe._pids_of', return_value={123}), \
             patch.object(Screen, '_write_config') as config, \
             patch('iphone_use_capture.screen_pipe.subprocess.Popen') as spawn:
            Screen(start_receiver=True)._ensure_receiver()
            config.assert_not_called()
            spawn.assert_not_called()

    def test_pipe_conflict_fails_before_receiver_mutation(self):
        os.environ.setdefault('UXPLAY_DIR', tempfile.gettempdir())
        from iphone_use_capture.screen_pipe import Screen
        with patch.object(Screen, '_create_pipe', side_effect=OSError('pipe occupied')), \
             patch.object(Screen, '_ensure_receiver') as receiver:
            with self.assertRaises(OSError):
                Screen(start_receiver=True).start()
            receiver.assert_not_called()
