# AgentSmith-HUB

> **Enterprise Security Data Pipeline Platform (SDPP) with Integrated Real-Time Threat Detection Engine**

AgentSmith-HUB is a **Security Data Pipeline Platform** designed to provide comprehensive security data processing, real-time threat detection, and automated incident response capabilities. It enables security teams to build, deploy, and manage sophisticated detection and response workflows at enterprise scale.

![Dashboard.png](docs/png/Dashboard.png)

If you have a lot of raw logs, security alarms that need to be processed, enriched, and linked with other systems, then the HUB will be your best tool to help you get the job done efficiently, standardized, and with very low learning costs.
![ExampleRule01.png](docs/png/ExampleRule01.png)

If you have an intrusion detection scenario, then the HUB can also support complex intrusion detection syntax, and the HUB has extremely high performance, which can easily and efficiently handle large amounts of data. And the HUB supports cluster mode, so you can easily expand capacity.
![ExampleRule02.png](docs/png/ExampleRule02.png)

The HUB not only has a powerful rules engine and plug-in mechanism, but also has a very flexible data orchestration mechanism, which can easily meet various needs in the workplace.
![ExampleProject.png](docs/png/ExampleProject.png)

### Function Show
Input Connect Check
![InputEditConnectCheck.gif](docs/GIF/InputEditConnectCheck.gif)

Rule Edit
![RuleEdit.gif](docs/GIF/RuleEdit.gif)

Rule Test
![RuleTest.gif](docs/GIF/RuleTest.gif)

Project Edit
![ProjectEdit.gif](docs/GIF/ProjectEdit.gif)

Plugin Test
![Plugintest.gif](docs/GIF/Plugintest.gif)

Search
![Search.gif](docs/GIF/Search.gif)

Errlog & Operations
![ErrlogOperations.gif](docs/GIF/ErrlogOperations.gif)

MCP
![MCP.png](docs/png/MCP.png)



### Guide
[Agentsmith-HUB Guide](docs/agentsmith-hub-guide.md)



### Performance Testing Report
[Performance Testing Report](docs/performance-testing-report.md)



### Deployment Tutorial

1. unzip and tar -xf AgentSmith-HUB, And make sure the hub folder is under /opt/: `/opt/agentsmith-hub`
2. Copy hub config folder to /opt/, `cp -r /opt/agentsmith-hub/config /opt/`
3. Install redis and configure it in `/opt/hub_config/config.yaml`
4. Run start.sh or stop.sh under the hub to start or stop the hub backend service, like: `./start.sh`, The start.sh default mode is Leader, `./start.sh --follower` will run in follower mode. In this mode, config.yaml needs to be consistent with Leader (that is, use the same Redis instance); For more information, see `./start.sh --help`.
5. The first time you run the backend, a token will be created, located in `/etc/hub/.token`
6. The backend logs are located in `/var/log/hub_logs/`
7. Install Nginx, and `sudo cp /opt/agentsmith-hub/nginx/nginx.conf /etc/nginx/`(This will overwrite your previous nginx.conf. Please back it up in advance if necessary), and run `sudo nginx -t reload`, the frontend will work on port 80 and a token is required for access.



### License

AgentSmith-HUB is licensed under the Apache License 2.0 with the Commons Clause restriction. This means:

- You are free to use, modify, and distribute the software for personal or non-commercial purposes.
- Commercial use, including direct or indirect integration into commercial products or services, is strictly prohibited.

For more details, see the [LICENSE](./LICENSE) file.

