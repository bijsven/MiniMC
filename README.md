<div align="center">
  <img src="./client/src/lib/assets/favicon.png" alt="Favicon" />

## MiniMC

Your lightweight Minecraft server companion!

</div>


#### What is MiniMC?

MiniMC is a **simple, self-contained Minecraft server manager** with a built-in web interface.

* Start your server by typing `start` in the terminal.
* Stop it with `stop`.
* If it crashes, use `kill`.
* Includes a **web-based file manager**, accessible via `/files` in the terminal.


#### Features

* Automatically downloads and runs the **latest PaperMC server build**.
* Single-container setup designed with **Docker** in mind.
* Lightweight, but includes **advanced logging** in the web interface.
* Self-contained: all server files are stored locally in `/minecraft`.


#### Getting Started

1. Clone the repository:

```bash
git clone https://github.com/bijsven/MiniMC.git
cd MiniMC
```

2. Configure Docker Compose to your needs. By default:

* Minecraft server exposed on `25565`
* Web interface exposed on `8080`
* Server files stored in `/minecraft` locally and internally within the container

3. Start MiniMC:

```bash
docker-compose up
```

4. Access the web interface via `http://localhost:8080` and manage your server easily.

##### Recommended: Use [Serveo](https://serveo.net/)
5. Enter your host terminal and type:
```bash
docker compose exec -it minimc /bin/bash
```
6. Then run:
```bash
ssh -R 25565:localhost:25565 -R 80:localhost:8080 serveo.net
```
7. Use the urls to access your MiniMC instant. Enjoy!

#### Usage Notes

* MiniMC **auto-updates** the PaperMC server jar whenever restarted.
* Use the terminal commands (`start`, `stop`, `kill`) to control the server.
* All server data is persistent inside the `/minecraft` folder, so you can safely restart or update the container.


#### License

MIT License – see [LICENSE](LICENSE)
