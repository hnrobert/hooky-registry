FROM python:3.11-slim
WORKDIR /app
COPY webhook_receiver.py /app/webhook_receiver.py
RUN pip install --no-cache-dir requests docker
EXPOSE 5001
CMD ["python", "/app/webhook_receiver.py"]
