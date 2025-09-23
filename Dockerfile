FROM python:3.11-alpine
WORKDIR /app
COPY requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r requirements.txt
COPY webhook_receiver.py /app/webhook_receiver.py
EXPOSE 5001
CMD ["python", "/app/webhook_receiver.py"]
