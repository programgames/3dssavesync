FROM python:3.13-slim
COPY server.py /app/server.py
ENV NDSSYNC_DATA=/data PYTHONUNBUFFERED=1
USER 1000:1000
EXPOSE 8080
CMD ["python", "/app/server.py"]
