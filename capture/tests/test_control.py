import unittest
from unittest.mock import Mock, patch

from iphone_use_capture.control import owned_process


class ProcessOwnership(unittest.TestCase):
    def test_reused_pid_is_rejected_before_inspecting_command(self):
        process = Mock()
        process.create_time.return_value = 20
        with patch('iphone_use_capture.control.psutil.Process', return_value=process):
            with self.assertRaisesRegex(RuntimeError, 'reused'):
                owned_process({'pid':123, 'created':10})
        process.terminate.assert_not_called()
        process.cmdline.assert_not_called()

    def test_other_python_module_is_not_owned(self):
        process = Mock()
        process.create_time.return_value = 10
        process.cmdline.return_value = ['python', '-m', 'unrelated_service']
        with patch('iphone_use_capture.control.psutil.Process', return_value=process):
            with self.assertRaisesRegex(RuntimeError, 'identity'):
                owned_process({'pid':123, 'created':10})
        process.terminate.assert_not_called()
