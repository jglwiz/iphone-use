import argparse
import logging
import os


def main():
    parser = argparse.ArgumentParser(description="iPhone Use independent AirPlay capture")
    parser.add_argument("--port", type=int, default=8892)
    parser.add_argument("--name", default="iPhone Use")
    parser.add_argument("--uxplay-dir", required=True)
    parser.add_argument("--data-dir", required=True)
    parser.add_argument("--receiver", choices=("attach", "start"), default="attach")
    args = parser.parse_args()
    from pathlib import Path
    for value in (args.uxplay_dir, args.data_dir):
        if not Path(value).is_absolute():
            parser.error("Paths must be absolute")
    os.environ["UXPLAY_DIR"] = args.uxplay_dir
    os.environ["HIDAGENT_DATA"] = args.data_dir
    from .screen_pipe import Screen
    from .capture_worker import serve
    logging.basicConfig(level=logging.INFO)
    serve(Screen(args.name, start_receiver=args.receiver == "start"), args.port)


if __name__ == "__main__":
    main()
