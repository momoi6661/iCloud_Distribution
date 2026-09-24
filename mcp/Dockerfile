FROM python:3.12-slim
WORKDIR /app
COPY mcp/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir -r requirements.txt
COPY mcp/server.py ./server.py
ENV MCP_TRANSPORT=streamable-http
ENV ICLOUD_DISTRIBUTION_URL=http://icloud_distribution:6981
EXPOSE 8787
CMD ["python", "server.py"]
