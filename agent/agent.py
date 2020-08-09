import os
import sys
import socket
import wmi
import servicemanager
import win32serviceutil
import configparser
import portalocker
import requests
import cv2
import flask
import pythoncom
import base64
import win32ts
import win32process
import win32con
import win32profile
import time
import threading


config_file_name = 'agent.ini'
config_section = 'agent'

if getattr(sys, 'frozen', False):
    application_path = os.path.dirname(sys.executable)
elif __file__:
    application_path = os.path.dirname(__file__)

config_file = open(os.path.join(application_path, config_file_name))

config = configparser.ConfigParser()
config.read_file(config_file)

config_file.seek(0)
portalocker.lock(config_file, portalocker.LOCK_EX)

overseer_host = config.get(config_section, 'overseer_host')
overseer_port = config.getint(config_section, 'overseer_port')


def logout():
    session_id = win32ts.WTSGetActiveConsoleSessionId()
    user_token = win32ts.WTSQueryUserToken(session_id)
    startup = win32process.STARTUPINFO()
    priority = win32con.NORMAL_PRIORITY_CLASS
    environment = win32profile.CreateEnvironmentBlock(user_token, False)
    win32process.CreateProcessAsUser(user_token, None, 'shutdown -l',
                                     None, None, True, priority, environment, None, startup)


def overseer_pinger(overseer_ip):
    while True:
        s = socket.socket()
        try:
            s.connect((overseer_ip, overseer_port))
        except:
            try:
                logout()
            except:
                pass
        s.close()
        time.sleep(1)


class AgentService(win32serviceutil.ServiceFramework):
    _svc_name_ = 'OkoAgent'
    _svc_display_name_ = 'Oko Agent'

    def SvcDoRun(self):
        overseer_ip = socket.gethostbyname(overseer_host)

        threading.Thread(target=overseer_pinger, args=[overseer_ip]).start()

        host_name = socket.gethostname()

        while True:
            try:
                res = requests.get('http://{}:{}/hosts/{}/agent_config'.format(overseer_host, overseer_port, host_name))
                if res.status_code != 200:
                    raise Exception('overseer returned not 200 status code')
            except:
                time.sleep(3)
                continue
            break

        agent_config = res.json()

        agent_host = agent_config['host']
        agent_port = agent_config['port']

        camera_id = agent_config['camera_id']

        def get_user_name():
            pythoncom.CoInitialize()
            user_name = None
            for process in wmi.WMI().Win32_Process(name='explorer.exe'):
                _, _, user_name = process.GetOwner()
                break
            return user_name


        app = flask.Flask(__name__)

        @app.route('/logout', methods=['POST'])
        def post_logout():
            if overseer_ip != flask.request.remote_addr:
                return ''

            logout()

            return ''

        @app.route('/status', methods=['GET'])
        def get_status():
            if overseer_ip != flask.request.remote_addr:
                return ''

            user_name = get_user_name()

            if user_name is None:
                return ''

            vc = cv2.VideoCapture(camera_id, cv2.CAP_DSHOW)
            for i in range(20):
                read, frame = vc.read()
            vc.release()

            user_name_encoded = base64.b64encode(user_name.encode("utf-8"))

            if not read:
                return flask.Response('unable to read frame', 500, {'X-Active-User': user_name_encoded})

            encoded, frame_jpg = cv2.imencode('.jpg', frame)
            if not encoded:
                return flask.Response('unable to JPEG encode frame', 500, {'X-Active-User': user_name_encoded})

            return flask.Response(frame_jpg.tobytes(), 200, {'X-Active-User': user_name_encoded})

        app.run(agent_host, agent_port)

    def SvcStop(self):
        pass


if __name__ == "__main__":
    if len(sys.argv) == 1:
        servicemanager.Initialize()
        servicemanager.PrepareToHostSingle(AgentService)
        servicemanager.StartServiceCtrlDispatcher()
    else:
        win32serviceutil.HandleCommandLine(AgentService)